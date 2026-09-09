package v8_5

import (
	"cosmossdk.io/store/types"
	"github.com/MANTRA-Chain/mantrachain/v8/app/upgrades"
)

const (
	// UpgradeName defines the on-chain upgrade name that activates the
	// governance execution timelock (x/govtimelock).
	UpgradeName = "v8.5.0"
)

var Upgrade = upgrades.Upgrade{
	UpgradeName:          UpgradeName,
	CreateUpgradeHandler: CreateUpgradeHandler,
	StoreUpgrades: types.StoreUpgrades{
		// x/govtimelock reuses the existing x/gov KVStore (see
		// x/govtimelock/types/keys.go), so no new store is added here.
		Added:   []string{},
		Deleted: []string{},
	},
}
