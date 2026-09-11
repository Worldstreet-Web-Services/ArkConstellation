package app

import (
	evmtypes "github.com/cosmos/evm/x/vm/types"

	"github.com/MANTRA-Chain/mantrachain/v8/app/precompiles/distrclaim"
)

// ArkActiveStaticPrecompiles is the set of EVM static precompile addresses Ark
// activates - in the shipped genesis files and in this binary's own
// DefaultGenesis. Decided in docs/decisions/module-and-config-decisions.md's
// "EVM Precompile Decisions" section; the per-precompile reasoning lives in
// docs/decisions/proposals/precompile-enablement-proposal.md.
//
//	0x...0100  P256          RIP-7212 signature verification, stateless
//	0x...0400  Bech32        address conversion, stateless
//	0x...0800  Staking       x/staking
//	0x...0801  Distribution  x/distribution
//	0x...0802  ICS20         ibc-go transfer
//	0x...0804  Bank          x/bank
//	0x...0805  Gov           x/gov
//	0x...0806  Slashing      x/slashing
//	0x...0a01  distrclaim    Ark-original, app/precompiles/distrclaim
//
// This list is written out rather than derived from
// evmtypes.AvailableStaticPrecompiles, and that is the entire reason this file
// exists. AvailableStaticPrecompiles includes VestingPrecompileAddress
// (0x...0803), but the fork ships no vesting precompile: precompiles/ has no
// vesting package and precompiletypes.DefaultStaticPrecompiles never registers
// one. Activating an address absent from the keeper's map makes
// keeper.GetStaticPrecompileInstance panic ("precompiled contract not stored in
// memory"), and an unauthenticated eth_call can reach that path. So copying
// AvailableStaticPrecompiles wholesale - which the fork's own evmd reference app
// does in evmd/genesis.go - activates a remotely triggerable panic. Don't.
// TestArkActiveStaticPrecompilesAllRegistered keeps this honest across a re-pin.
//
// Two properties are load-bearing and both are asserted in precompiles_test.go:
// the slice must be sorted (evmtypes.ValidatePrecompiles rejects unsorted input,
// though SetParams quietly sorts first), and the addresses must stay lowercase -
// IsAvailableStaticPrecompile compares against common.Address.String(), which is
// EIP-55 checksummed, and every address here checksums to its lowercase form.
func ArkActiveStaticPrecompiles() []string {
	return []string{
		evmtypes.P256PrecompileAddress,
		evmtypes.Bech32PrecompileAddress,
		evmtypes.StakingPrecompileAddress,
		evmtypes.DistributionPrecompileAddress,
		evmtypes.ICS20PrecompileAddress,
		evmtypes.BankPrecompileAddress,
		evmtypes.GovPrecompileAddress,
		evmtypes.SlashingPrecompileAddress,
		distrclaim.DistributionClaimPrecompileAddress,
	}
}
