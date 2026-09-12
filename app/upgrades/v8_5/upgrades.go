package v8_5

import (
	"context"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/MANTRA-Chain/mantrachain/v8/app/upgrades"
	"github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock"
	govtimelocktypes "github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
)

// CreateUpgradeHandler activates the 48-hour governance execution timelock.
//
// The timelock changes what EndBlock does for passed proposals (immediate
// execution becomes deferred execution), which is a consensus-behavior change.
// It must therefore only take effect at a coordinated upgrade height rather
// than whenever an operator happens to swap binaries: a node on the old binary
// executing a proposal immediately while a node on the new binary only
// schedules it is an instant AppHash divergence.
//
// The handler records the activation height, and x/govtimelock's EndBlocker
// stays inert until the chain reaches it.
func CreateUpgradeHandler(
	mm *module.Manager,
	configurator module.Configurator,
	keepers *upgrades.UpgradeKeepers,
	storekeys map[string]*storetypes.KVStoreKey,
) upgradetypes.UpgradeHandler {
	return func(c context.Context, plan upgradetypes.Plan, vm module.VersionMap) (module.VersionMap, error) {
		ctx := sdk.UnwrapSDKContext(c)
		ctx.Logger().Info("Starting v8.5.0 upgrade...")

		// govtimelock is a brand-new module, so RunMigrations would otherwise call
		// its InitGenesis with an empty DefaultGenesis(): that cross-validates
		// against x/gov and panics on any proposal that has ever passed on this
		// chain. Activation for existing chains happens explicitly below via
		// SetActivationHeight instead, so pre-seed the version map to make
		// RunMigrations treat govtimelock as already initialized and skip its
		// InitGenesis entirely.
		if keepers.GovTimelockKeeper != nil {
			vm[govtimelocktypes.ModuleName] = govtimelock.ConsensusVersion
		}

		ctx.Logger().Info("Running module migrations...")
		vm, err := mm.RunMigrations(ctx, configurator, vm)
		if err != nil {
			return vm, err
		}

		if keepers.GovTimelockKeeper != nil {
			height := ctx.BlockHeight()
			if err := keepers.GovTimelockKeeper.SetActivationHeight(ctx, height); err != nil {
				return vm, err
			}
			ctx.Logger().Info(
				"governance execution timelock activated",
				"height", height,
				"delay", govtimelocktypes.MinimumDelay,
			)
		}

		ctx.Logger().Info("Upgrade v8.5.0 complete")
		return vm, nil
	}
}
