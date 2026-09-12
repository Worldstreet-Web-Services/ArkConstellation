package types

import (
	"fmt"
	"time"
)

type ScheduledProposal struct {
	ProposalID    uint64    `json:"proposal_id"`
	ExecutionTime time.Time `json:"execution_time"`
}

type GenesisState struct {
	ScheduledProposals []ScheduledProposal `json:"scheduled_proposals"`
	// ActivationHeight is the height from which the execution timelock governs
	// proposal execution. A chain starting fresh from genesis sets 1 so the
	// timelock is on from the first block; an existing chain leaves it 0 and
	// lets the coordinated upgrade handler set it, so that old and new binaries
	// agree on behavior until the upgrade height.
	ActivationHeight int64 `json:"activation_height"`
	// ExecutionDelay is how long a passed proposal waits before its messages
	// run. Zero means the module default (MinimumDelay, 48h). It is genesis
	// state rather than a compile-time constant so test networks can use a
	// short delay; production genesis must leave it unset or set it to at
	// least MinimumDelay, which Validate enforces via the `genesis
	// validate-genesis` CLI path (AppModuleBasic.ValidateGenesis) — not via
	// InitGenesis, which intentionally skips this check so e2e/interchain test
	// genesis (a real genesis.json fed to InitChain, not just the CLI) can use
	// a short delay without a 48-real-hour wait.
	ExecutionDelay time.Duration `json:"execution_delay"`
}

// DefaultGenesis activates the timelock from the first block, which is the
// right default for a new chain: a genesis that left it unset would run with no
// timelock at all until someone remembered to schedule an upgrade.
func DefaultGenesis() GenesisState {
	return GenesisState{ScheduledProposals: []ScheduledProposal{}, ActivationHeight: 1}
}

func (gs GenesisState) Validate() error {
	if gs.ExecutionDelay < 0 {
		return fmt.Errorf("execution delay must not be negative, got %s", gs.ExecutionDelay)
	}
	if gs.ExecutionDelay != 0 && gs.ExecutionDelay < MinimumDelay {
		return fmt.Errorf("execution delay must be at least %s, got %s", MinimumDelay, gs.ExecutionDelay)
	}
	if gs.ActivationHeight < 0 {
		return fmt.Errorf("activation height must not be negative, got %d", gs.ActivationHeight)
	}
	if gs.ActivationHeight == 0 && len(gs.ScheduledProposals) > 0 {
		return fmt.Errorf("timelock is inactive (activation height 0) but %d proposals are scheduled", len(gs.ScheduledProposals))
	}
	seen := make(map[uint64]struct{}, len(gs.ScheduledProposals))
	for _, scheduled := range gs.ScheduledProposals {
		if scheduled.ProposalID == 0 {
			return fmt.Errorf("scheduled proposal ID must be positive")
		}
		if scheduled.ExecutionTime.IsZero() {
			return fmt.Errorf("proposal %d has zero execution time", scheduled.ProposalID)
		}
		if _, ok := seen[scheduled.ProposalID]; ok {
			return fmt.Errorf("proposal %d is scheduled more than once", scheduled.ProposalID)
		}
		seen[scheduled.ProposalID] = struct{}{}
	}
	return nil
}
