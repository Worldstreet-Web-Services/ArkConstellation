package types_test

import (
	"testing"
	"time"

	"github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock/types"
	"github.com/stretchr/testify/require"
)

func TestGenesisValidation(t *testing.T) {
	now := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		state   types.GenesisState
		wantErr string
	}{
		{name: "empty", state: types.DefaultGenesis()},
		{name: "valid", state: types.GenesisState{ScheduledProposals: []types.ScheduledProposal{{ProposalID: 1, ExecutionTime: now}}}},
		{name: "zero proposal ID", state: types.GenesisState{ScheduledProposals: []types.ScheduledProposal{{ExecutionTime: now}}}, wantErr: "must be positive"},
		{name: "zero execution time", state: types.GenesisState{ScheduledProposals: []types.ScheduledProposal{{ProposalID: 1}}}, wantErr: "zero execution time"},
		{name: "duplicate proposal", state: types.GenesisState{ScheduledProposals: []types.ScheduledProposal{{ProposalID: 1, ExecutionTime: now}, {ProposalID: 1, ExecutionTime: now.Add(time.Hour)}}}, wantErr: "more than once"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.state.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
