package app

import (
	evmtypes "github.com/cosmos/evm/x/vm/types"

	"github.com/MANTRA-Chain/mantrachain/v8/app/precompiles/distrclaim"
)

// StaticPrecompileAddresses is every static precompile the app registers with
// the EVM keeper (corePrecompiles in New). Every genesis fixture under
// networks/ and scripts/genesis/ must activate exactly this set;
// TestStaticPrecompileFixtures enforces it against the live keeper.
//
// evmtypes.AvailableStaticPrecompiles is deliberately not used: it still lists
// the vesting precompile (0x...0803) that this cosmos/evm version no longer
// ships, and activating an unregistered address panics on first call.
var StaticPrecompileAddresses = []string{
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
