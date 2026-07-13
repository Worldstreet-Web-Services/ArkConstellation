package v8_3

import (
	"github.com/MANTRA-Chain/mantrachain/v8/app/upgrades"
	"github.com/cosmos/cosmos-sdk/store/v2/types"
)

const (
	// UpgradeName defines the on-chain upgrade name.
	UpgradeName = "v8.3.0"
)

var Upgrade = upgrades.Upgrade{
	UpgradeName:          UpgradeName,
	CreateUpgradeHandler: CreateUpgradeHandler,
	// No store changes: the sdk v0.54 / ibc-go v11 / evm v0.7 bump adds only
	// non-persisted object stores and removes transient stores, neither of
	// which is part of the committed multistore.
	StoreUpgrades: types.StoreUpgrades{
		Added:   []string{},
		Deleted: []string{},
	},
}
