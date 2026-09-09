package types

import (
	"context"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// BankKeeper is the subset of the bank keeper this module needs.
//
// Deliberately narrow: metadata read and write only. No balance access, no
// minting, no burning. The module cannot move or create tokens even if its
// authority were compromised — it can only relabel denominations that already
// exist.
//
// Signatures take context.Context rather than sdk.Context to match the SDK's
// bank keeper as of v0.53.
type BankKeeper interface {
	GetDenomMetaData(ctx context.Context, denom string) (banktypes.Metadata, bool)
	SetDenomMetaData(ctx context.Context, denomMetaData banktypes.Metadata)
}
