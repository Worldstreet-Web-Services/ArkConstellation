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
		{name: "valid", state: types.GenesisState{ActivationHeight: 1, ScheduledProposals: []types.ScheduledProposal{{ProposalID: 1, ExecutionTime: now}}}},
		{name: "zero proposal ID", state: types.GenesisState{ActivationHeight: 1, ScheduledProposals: []types.ScheduledProposal{{ExecutionTime: now}}}, wantErr: "must be positive"},
		{name: "zero execution time", state: types.GenesisState{ActivationHeight: 1, ScheduledProposals: []types.ScheduledProposal{{ProposalID: 1}}}, wantErr: "zero execution time"},
		{name: "duplicate proposal", state: types.GenesisState{ActivationHeight: 1, ScheduledProposals: []types.ScheduledProposal{{ProposalID: 1, ExecutionTime: now}, {ProposalID: 1, ExecutionTime: now.Add(time.Hour)}}}, wantErr: "more than once"},
		{name: "default genesis activates from first block", state: types.DefaultGenesis()},
		{name: "negative activation height", state: types.GenesisState{ActivationHeight: -1}, wantErr: "must not be negative"},
		{name: "inactive with schedules", state: types.GenesisState{ActivationHeight: 0, ScheduledProposals: []types.ScheduledProposal{{ProposalID: 1, ExecutionTime: now}}}, wantErr: "timelock is inactive"},
		{name: "inactive and empty is valid for an upgrading chain", state: types.GenesisState{ActivationHeight: 0}},
		{name: "negative execution delay", state: types.GenesisState{ExecutionDelay: -1}, wantErr: "must not be negative"},
		{name: "zero execution delay uses module default", state: types.GenesisState{ExecutionDelay: 0}},
		{name: "execution delay below minimum", state: types.GenesisState{ExecutionDelay: types.MinimumDelay - time.Nanosecond}, wantErr: "must be at least"},
		{name: "execution delay at minimum", state: types.GenesisState{ExecutionDelay: types.MinimumDelay}},
		{name: "execution delay above minimum", state: types.GenesisState{ExecutionDelay: types.MinimumDelay + time.Hour}},
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
