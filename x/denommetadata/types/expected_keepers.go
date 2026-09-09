package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// BankKeeper is the subset of the bank keeper this module needs.
//
// Deliberately narrow: metadata read and write only. No balance access, no
// minting, no burning. The module cannot move or create tokens even if its
// authority were compromised — it can only relabel denominations that already
// exist.
type BankKeeper interface {
	GetDenomMetaData(ctx sdk.Context, denom string) (banktypes.Metadata, bool)
	SetDenomMetaData(ctx sdk.Context, denomMetaData banktypes.Metadata)
}
