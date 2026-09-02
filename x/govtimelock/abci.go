package govtimelock

import (
	"errors"
	"fmt"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/log"
	"github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock/keeper"
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
func EndBlocker(ctx sdk.Context, govKeeper *govkeeper.Keeper, timelockKeeper keeper.Keeper, delay time.Duration) error {
	defer telemetry.ModuleMeasureSince(govtypes.ModuleName, telemetry.Now(), telemetry.MetricKeyEndBlocker)

	logger := ctx.Logger().With("module", "x/"+govtypes.ModuleName)
	if err := processInactiveProposals(ctx, govKeeper, logger); err != nil {
		return err
	}
	if err := processEndedVotingPeriods(ctx, govKeeper, timelockKeeper, delay, logger); err != nil {
		return err
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
			logger.Info("proposal did not meet minimum deposit; deleted", "proposal", proposal.Id)
		}
	}
	return nil
}

func processEndedVotingPeriods(
	ctx sdk.Context,
	govKeeper *govkeeper.Keeper,
	timelockKeeper keeper.Keeper,
	delay time.Duration,
	logger log.Logger,
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
		switch {
		case passes:
			executionTime := ctx.BlockTime().Add(delay)
			if err := timelockKeeper.Schedule(ctx, proposal.Id, executionTime); err != nil {
				return err
			}
			proposal.Status = govv1.StatusPassed
			proposal.FailedReason = ""
			tagValue = govtypes.AttributeValueProposalPassed
			logMsg = fmt.Sprintf("passed; execution scheduled for %s", executionTime.UTC().Format(time.RFC3339Nano))
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

		cacheCtx, writeCache := ctx.CacheContext()
		err = govKeeper.Hooks().AfterProposalVotingPeriodEnded(cacheCtx, proposal.Id)
		if err == nil {
			writeCache()
		} else {
			govKeeper.Logger(ctx).Error("failed to execute AfterProposalVotingPeriodEnded hook", "error", err)
		}

		logger.Info("proposal tallied", "proposal", proposal.Id, "results", logMsg)
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
		proposal, err := govKeeper.Proposals.Get(ctx, scheduled.ProposalID)
		if err != nil {
			return err
		}
		if proposal.Status != govv1.StatusPassed {
			return fmt.Errorf("scheduled proposal %d has unexpected status %s", proposal.Id, proposal.Status.String())
		}

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

		result := govtypes.AttributeValueProposalPassed
		if err != nil {
			proposal.Status = govv1.StatusFailed
			proposal.FailedReason = err.Error()
			result = govtypes.AttributeValueProposalFailed
			logger.Error("timelocked proposal failed to execute", "proposal", proposal.Id, "error", err)
		} else {
			proposal.FailedReason = ""
			logger.Info("timelocked proposal executed", "proposal", proposal.Id)
		}
		if err := govKeeper.SetProposal(ctx, proposal); err != nil {
			return err
		}
		if err := timelockKeeper.Remove(ctx, scheduled.ExecutionTime, scheduled.ProposalID); err != nil {
			return err
		}
		ctx.EventManager().EmitEvent(sdk.NewEvent(
			eventTypeProposalExecuted,
			sdk.NewAttribute(govtypes.AttributeKeyProposalID, fmt.Sprintf("%d", proposal.Id)),
			sdk.NewAttribute(govtypes.AttributeKeyProposalResult, result),
		))
	}
	return nil
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
	logger.Info("proposal failed to decode; deleted", "proposal", proposal.Id, "results", reason)
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		eventType,
		sdk.NewAttribute(govtypes.AttributeKeyProposalID, fmt.Sprintf("%d", proposal.Id)),
		sdk.NewAttribute(govtypes.AttributeKeyProposalResult, govtypes.AttributeValueProposalFailed),
		sdk.NewAttribute(govtypes.AttributeKeyProposalLog, reason),
	))
	return nil
}
