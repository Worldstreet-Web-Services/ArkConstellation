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
}

func DefaultGenesis() GenesisState {
	return GenesisState{ScheduledProposals: []ScheduledProposal{}}
}

func (gs GenesisState) Validate() error {
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
