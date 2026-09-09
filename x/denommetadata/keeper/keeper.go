package keeper

import (
	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/codec"

	"github.com/MANTRA-Chain/mantrachain/v8/x/denommetadata/types"
)

// Keeper writes denom metadata into x/bank under governance authority.
//
// It owns no state of its own — no store key, no genesis. x/bank remains the
// single source of truth for denom metadata; this module is only a
// gov-gated write path to it, because x/bank exposes no message for the job
// and x/tokenfactory (which does) was removed from this chain.
type Keeper struct {
	cdc        codec.BinaryCodec
	logger     log.Logger
	bankKeeper types.BankKeeper

	// authority is the only address permitted to set metadata; normally the
	// gov module account.
	authority string
}

func NewKeeper(
	cdc codec.BinaryCodec,
	logger log.Logger,
	bankKeeper types.BankKeeper,
	authority string,
) Keeper {
	return Keeper{
		cdc:        cdc,
		logger:     logger.With("module", "x/"+types.ModuleName),
		bankKeeper: bankKeeper,
		authority:  authority,
	}
}

// GetAuthority returns the module's authority.
func (k Keeper) GetAuthority() string { return k.authority }

// Logger returns the module logger.
func (k Keeper) Logger() log.Logger { return k.logger }
