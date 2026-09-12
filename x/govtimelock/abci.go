package govtimelock

import (
	"errors"
	"fmt"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/log"
	"github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock/keeper"
	"github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
)

const (
	eventTypeProposalScheduled = "proposal_scheduled"
	eventTypeProposalExecuted  = "proposal_executed"
	attributeKeyExecutionTime  = "execution_time"
)

// EndBlocker retains the stock x/gov tally behavior but schedules passed
// proposal messages for atomic execution after delay instead of executing
// them in the voting-period end block.
//
// Status semantics caveat: a scheduled proposal is set to
// PROPOSAL_STATUS_PASSED at tally time, which the SDK documents as "passed and
// successfully executed". Under the timelock that enum value additionally
// covers the pending-execution window — up to the configured delay during which
// the proposal has passed but its messages have NOT run. Off-the-shelf
// consumers (indexers, explorers, wallets, and other chains reading v1 status
// over IBC/ICS) will report such a proposal as executed for the whole window.
// Consumers that need to distinguish the two states must watch the
// proposal_scheduled and proposal_executed events this module emits; only those
// events separate "scheduled" from "executed". Introducing a new status value
// would be the cleaner fix but is a v1 wire-format break, so it is deliberately
// not done here — see docs/proof/gov-timelock.md.
func EndBlocker(ctx sdk.Context, govKeeper *govkeeper.Keeper, timelockKeeper keeper.Keeper, fallbackDelay time.Duration) error {
	defer telemetry.ModuleMeasureSince(govtypes.ModuleName, telemetry.Now(), telemetry.MetricKeyEndBlocker)

	logger := ctx.Logger().With("module", "x/"+govtypes.ModuleName)

	// The timelock is a consensus-behavior change, so it must only engage at the
	// coordinated upgrade height recorded by the v8.5.0 handler. Before that
	// height every node — old binary or new — runs stock x/gov execution
	// semantics, which is what keeps a plain binary swap from forking the chain.
	active, err := timelockKeeper.IsActive(ctx, ctx.BlockHeight())
	if err != nil {
		return err
	}

	if err := processInactiveProposals(ctx, govKeeper, logger); err != nil {
		return err
	}
	if err := processEndedVotingPeriods(ctx, govKeeper, timelockKeeper, fallbackDelay, logger, active); err != nil {
		return err
	}
	if !active {
		return nil
	}
	return executeMaturedProposals(ctx, govKeeper, timelockKeeper, logger)
}

func processInactiveProposals(ctx sdk.Context, govKeeper *govkeeper.Keeper, logger log.Logger) error {
	rng := collections.NewPrefixUntilPairRange[time.Time, uint64](ctx.BlockTime())
	iter, err := govKeeper.InactiveProposalsQueue.Iterate(ctx, rng)
	if err != nil {
		return err
	}
	inactiveProps, err := iter.KeyValues()
	if err != nil {
		return err
	}

	for _, prop := range inactiveProps {
		proposal, err := govKeeper.Proposals.Get(ctx, prop.Key.K2())
		switch {
		case errors.Is(err, collections.ErrEncoding):
			proposal.Id = prop.Key.K2()
			if err := failUnsupportedProposal(logger, ctx, govKeeper, proposal, err.Error(), false); err != nil {
				return err
			}
			if err := govKeeper.InactiveProposalsQueue.Remove(ctx, prop.Key); err != nil {
				return err
			}
			if err := govKeeper.DeleteProposal(ctx, proposal.Id); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if err = govKeeper.DeleteProposal(ctx, proposal.Id); err != nil {
				return err
			}
			params, err := govKeeper.Params.Get(ctx)
			if err != nil {
				return err
			}
			if !params.BurnProposalDepositPrevote {
				err = govKeeper.RefundAndDeleteDeposits(ctx, proposal.Id)
			} else {
				err = govKeeper.DeleteAndBurnDeposits(ctx, proposal.Id)
			}
			if err != nil {
				return err
			}

			cacheCtx, writeCache := ctx.CacheContext()
			err = govKeeper.Hooks().AfterProposalFailedMinDeposit(cacheCtx, proposal.Id)
			if err == nil {
				writeCache()
			} else {
				govKeeper.Logger(ctx).Error("failed to execute AfterProposalFailedMinDeposit hook", "error", err)
			}

			ctx.EventManager().EmitEvent(sdk.NewEvent(
				govtypes.EventTypeInactiveProposal,
				sdk.NewAttribute(govtypes.AttributeKeyProposalID, fmt.Sprintf("%d", proposal.Id)),
				sdk.NewAttribute(govtypes.AttributeKeyProposalResult, govtypes.AttributeValueProposalDropped),
			))
			logger.Info(
				"proposal did not meet minimum deposit; deleted",
				"proposal", proposal.Id,
				"expedited", proposal.Expedited,
				"title", proposal.Title,
				"min_deposit", sdk.NewCoins(proposal.GetMinDepositFromParams(params)...).String(),
				"total_deposit", sdk.NewCoins(proposal.TotalDeposit...).String(),
			)
		}
	}
	return nil
}

func processEndedVotingPeriods(
	ctx sdk.Context,
	govKeeper *govkeeper.Keeper,
	timelockKeeper keeper.Keeper,
	fallbackDelay time.Duration,
	logger log.Logger,
	active bool,
) error {
	rng := collections.NewPrefixUntilPairRange[time.Time, uint64](ctx.BlockTime())
	iter, err := govKeeper.ActiveProposalsQueue.Iterate(ctx, rng)
	if err != nil {
		return err
	}
	activeProps, err := iter.KeyValues()
	if err != nil {
		return err
	}

	for _, prop := range activeProps {
		proposal, err := govKeeper.Proposals.Get(ctx, prop.Key.K2())
		switch {
		case errors.Is(err, collections.ErrEncoding):
			proposal.Id = prop.Key.K2()
			if err := failUnsupportedProposal(logger, ctx, govKeeper, proposal, err.Error(), true); err != nil {
				return err
			}
			if err := govKeeper.ActiveProposalsQueue.Remove(ctx, prop.Key); err != nil {
				return err
			}
			continue
		case err != nil:
			return err
		}

		passes, burnDeposits, tallyResults, err := govKeeper.Tally(ctx, proposal)
		if err != nil {
			return err
		}
		if !proposal.Expedited || passes {
			if burnDeposits {
				err = govKeeper.DeleteAndBurnDeposits(ctx, proposal.Id)
			} else {
				err = govKeeper.RefundAndDeleteDeposits(ctx, proposal.Id)
			}
			if err != nil {
				return err
			}
		}
		if err = govKeeper.ActiveProposalsQueue.Remove(ctx, prop.Key); err != nil {
			return err
		}

		var tagValue, logMsg string
		scheduledForLater := false
		switch {
		case passes && !active:
			// Pre-activation: stock x/gov semantics — execute in this block.
			proposal, tagValue, logMsg = executeProposal(ctx, govKeeper, proposal, logger)
		case passes:
			delay, err := timelockKeeper.GetExecutionDelay(ctx, fallbackDelay)
			if err != nil {
				return err
			}
			executionTime := ctx.BlockTime().Add(delay)
			if err := timelockKeeper.Schedule(ctx, proposal.Id, executionTime); err != nil {
				return err
			}
			proposal.Status = govv1.StatusPassed
			proposal.FailedReason = ""
			tagValue = govtypes.AttributeValueProposalPassed
			logMsg = fmt.Sprintf("passed; execution scheduled for %s", executionTime.UTC().Format(time.RFC3339Nano))
			scheduledForLater = true
			ctx.EventManager().EmitEvent(sdk.NewEvent(
				eventTypeProposalScheduled,
				sdk.NewAttribute(govtypes.AttributeKeyProposalID, fmt.Sprintf("%d", proposal.Id)),
				sdk.NewAttribute(attributeKeyExecutionTime, executionTime.UTC().Format(time.RFC3339Nano)),
			))
		case proposal.Expedited:
			proposal.Expedited = false
			params, err := govKeeper.Params.Get(ctx)
			if err != nil {
				return err
			}
			endTime := proposal.VotingStartTime.Add(*params.VotingPeriod)
			proposal.VotingEndTime = &endTime
			if err = govKeeper.ActiveProposalsQueue.Set(ctx, collections.Join(endTime, proposal.Id), proposal.Id); err != nil {
				return err
			}
			tagValue = govtypes.AttributeValueExpeditedProposalRejected
			logMsg = "expedited proposal converted to regular"
		default:
			proposal.Status = govv1.StatusRejected
			proposal.FailedReason = "proposal did not get enough votes to pass"
			tagValue = govtypes.AttributeValueProposalRejected
			logMsg = "rejected"
		}

		proposal.FinalTallyResult = &tallyResults
		if err = govKeeper.SetProposal(ctx, proposal); err != nil {
			return err
		}

		// AfterProposalVotingPeriodEnded fires here for every outcome except a
		// scheduled proposal. Stock x/gov fires it only after the proposal's
		// messages have run, so for the timelocked path it is deferred to
		// executeMaturedProposals to preserve that ordering for hook consumers.
		if !scheduledForLater {
			runVotingPeriodEndedHook(ctx, govKeeper, proposal.Id)
		}

		logger.Info(
			"proposal tallied",
			"proposal", proposal.Id,
			"status", proposal.Status.String(),
			"expedited", proposal.Expedited,
			"title", proposal.Title,
			"results", logMsg,
		)
		ctx.EventManager().EmitEvent(sdk.NewEvent(
			govtypes.EventTypeActiveProposal,
			sdk.NewAttribute(govtypes.AttributeKeyProposalID, fmt.Sprintf("%d", proposal.Id)),
			sdk.NewAttribute(govtypes.AttributeKeyProposalResult, tagValue),
			sdk.NewAttribute(govtypes.AttributeKeyProposalLog, logMsg),
		))
	}
	return nil
}

func executeMaturedProposals(ctx sdk.Context, govKeeper *govkeeper.Keeper, timelockKeeper keeper.Keeper, logger log.Logger) error {
	due, err := timelockKeeper.Due(ctx, ctx.BlockTime())
	if err != nil {
		return err
	}
	for _, scheduled := range due {
		// A single malformed or unexpected scheduled entry must never abort
		// EndBlock: returning an error here is fatal for the block, and because
		// the entry is only dropped by the Remove below, it would still be due
		// on every subsequent block — a permanent chain halt. Mirror the
		// per-proposal recovery the sibling loops in this file already use:
		// drop the offending entry and carry on with the rest.
		proposal, err := govKeeper.Proposals.Get(ctx, scheduled.ProposalID)
		if err != nil {
			logger.Error(
				"dropping scheduled proposal that could not be loaded",
				"proposal", scheduled.ProposalID,
				"error", err,
			)
			if err := dropScheduledEntry(ctx, timelockKeeper, scheduled, "scheduled proposal could not be loaded"); err != nil {
				return err
			}
			continue
		}
		if proposal.Status != govv1.StatusPassed {
			// Something outside this module moved the proposal off StatusPassed
			// during the delay window. Executing it now would be wrong, so drop
			// the schedule and leave the proposal record as-is.
			logger.Error(
				"dropping scheduled proposal with unexpected status",
				"proposal", proposal.Id,
				"status", proposal.Status.String(),
			)
			reason := fmt.Sprintf("unexpected status %s at execution time", proposal.Status.String())
			if err := dropScheduledEntry(ctx, timelockKeeper, scheduled, reason); err != nil {
				return err
			}
			continue
		}

		proposal, result, _ := executeProposal(ctx, govKeeper, proposal, logger)

		if err := govKeeper.SetProposal(ctx, proposal); err != nil {
			return err
		}
		if err := timelockKeeper.Remove(ctx, scheduled.ExecutionTime, scheduled.ProposalID); err != nil {
			return err
		}
		// Deferred from processEndedVotingPeriods so hook consumers observe the
		// same "fires after execution" ordering stock x/gov gives them.
		runVotingPeriodEndedHook(ctx, govKeeper, proposal.Id)
		ctx.EventManager().EmitEvent(sdk.NewEvent(
			eventTypeProposalExecuted,
			sdk.NewAttribute(govtypes.AttributeKeyProposalID, fmt.Sprintf("%d", proposal.Id)),
			sdk.NewAttribute(govtypes.AttributeKeyProposalResult, result),
		))
	}
	return nil
}

// dropScheduledEntry removes a matured schedule entry that cannot be executed
// and emits the corresponding failed proposal_executed event, so the two
// recovery branches in executeMaturedProposals can't drift apart.
func dropScheduledEntry(ctx sdk.Context, timelockKeeper keeper.Keeper, scheduled types.ScheduledProposal, reason string) error {
	if err := timelockKeeper.Remove(ctx, scheduled.ExecutionTime, scheduled.ProposalID); err != nil {
		return err
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		eventTypeProposalExecuted,
		sdk.NewAttribute(govtypes.AttributeKeyProposalID, fmt.Sprintf("%d", scheduled.ProposalID)),
		sdk.NewAttribute(govtypes.AttributeKeyProposalResult, govtypes.AttributeValueProposalFailed),
		sdk.NewAttribute(govtypes.AttributeKeyProposalLog, reason),
	))
	return nil
}

// runVotingPeriodEndedHook invokes the governance hook against a cache context,
// keeping a failing hook from aborting the block — matching stock x/gov.
func runVotingPeriodEndedHook(ctx sdk.Context, govKeeper *govkeeper.Keeper, proposalID uint64) {
	cacheCtx, writeCache := ctx.CacheContext()
	if err := govKeeper.Hooks().AfterProposalVotingPeriodEnded(cacheCtx, proposalID); err == nil {
		writeCache()
	} else {
		govKeeper.Logger(ctx).Error("failed to execute AfterProposalVotingPeriodEnded hook", "error", err)
	}
}

// executeProposal runs a passed proposal's messages atomically against a cache
// context, writing state only if every message succeeds. Shared by the
// pre-activation (stock semantics) and post-activation (timelocked) paths so
// the two cannot drift apart.
func executeProposal(ctx sdk.Context, govKeeper *govkeeper.Keeper, proposal govv1.Proposal, logger log.Logger) (govv1.Proposal, string, string) {
	cacheCtx, writeCache := ctx.CacheContext()
	messages, err := proposal.GetMsgs()
	if err == nil {
		var events sdk.Events
		for idx, msg := range messages {
			handler := govKeeper.Router().Handler(msg)
			res, execErr := safeExecuteHandler(cacheCtx, msg, handler)
			if execErr != nil {
				err = fmt.Errorf("message %d (%s): %w", idx, sdk.MsgTypeURL(msg), execErr)
				break
			}
			events = append(events, res.GetEvents()...)
		}
		if err == nil {
			writeCache()
			ctx.EventManager().EmitEvents(events)
		}
	}

	if err != nil {
		proposal.Status = govv1.StatusFailed
		proposal.FailedReason = err.Error()
		logger.Error("proposal failed to execute", "proposal", proposal.Id, "error", err)
		return proposal, govtypes.AttributeValueProposalFailed, "passed, but failed on execution: " + err.Error()
	}
	proposal.Status = govv1.StatusPassed
	proposal.FailedReason = ""
	logger.Info("proposal executed", "proposal", proposal.Id)
	return proposal, govtypes.AttributeValueProposalPassed, "passed"
}

func safeExecuteHandler(ctx sdk.Context, msg sdk.Msg, handler baseapp.MsgServiceHandler) (res *sdk.Result, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("handling x/gov proposal msg [%s] panicked: %v", msg, recovered)
		}
	}()
	if handler == nil {
		return nil, fmt.Errorf("no handler for proposal message %s", sdk.MsgTypeURL(msg))
	}
	return handler(ctx, msg)
}

func failUnsupportedProposal(
	logger log.Logger,
	ctx sdk.Context,
	govKeeper *govkeeper.Keeper,
	proposal govv1.Proposal,
	reason string,
	active bool,
) error {
	proposal.Status = govv1.StatusFailed
	proposal.FailedReason = fmt.Sprintf("proposal failed because it cannot be processed by gov: %s", reason)
	proposal.Messages = nil
	if err := govKeeper.SetProposal(ctx, proposal); err != nil {
		return err
	}
	if err := govKeeper.RefundAndDeleteDeposits(ctx, proposal.Id); err != nil {
		return err
	}
	eventType := govtypes.EventTypeInactiveProposal
	if active {
		eventType = govtypes.EventTypeActiveProposal
	}
	logger.Info(
		"proposal failed to decode; deleted",
		"proposal", proposal.Id,
		"expedited", proposal.Expedited,
		"title", proposal.Title,
		"results", reason,
	)
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		eventType,
		sdk.NewAttribute(govtypes.AttributeKeyProposalID, fmt.Sprintf("%d", proposal.Id)),
		sdk.NewAttribute(govtypes.AttributeKeyProposalResult, govtypes.AttributeValueProposalFailed),
		sdk.NewAttribute(govtypes.AttributeKeyProposalLog, reason),
	))
	return nil
}
