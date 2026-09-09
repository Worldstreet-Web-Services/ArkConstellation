package keeper

import (
	"context"

	sdkerrors "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/MANTRA-Chain/mantrachain/v8/x/denommetadata/types"
)

type msgServer struct{ Keeper }

// NewMsgServerImpl returns a MsgServer implementation.
func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{Keeper: k} }

var _ types.MsgServer = msgServer{}

// SetDenomMetadata writes bank denom metadata for denominations that already
// exist on this chain.
//
// Two deliberate restrictions, both enforced here rather than by convention:
//
//   - Only the authority (gov) may call it. Metadata drives how every wallet,
//     explorer and bridge interprets an amount, so a wrong exponent is a
//     financial hazard, not a cosmetic one.
//
//   - The denomination must already have metadata. A denom with none has never
//     been received or created here, and writing metadata for it would invent a
//     token that does not exist. This module corrects; it does not create.
func (k msgServer) SetDenomMetadata(
	goCtx context.Context,
	msg *types.MsgSetDenomMetadata,
) (*types.MsgSetDenomMetadataResponse, error) {
	if k.authority != msg.Authority {
		return nil, sdkerrors.Wrapf(types.ErrInvalidAuthority,
			"expected %s, got %s", k.authority, msg.Authority)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)

	for _, md := range msg.Metadata {
		existing, found := k.bankKeeper.GetDenomMetaData(ctx, md.Base)
		if !found {
			return nil, sdkerrors.Wrapf(types.ErrDenomNotFound,
				"denom %q has no existing metadata on this chain; this module corrects "+
					"metadata for denominations that exist, it does not create them", md.Base)
		}

		k.bankKeeper.SetDenomMetaData(ctx, md)

		k.Logger().Info("denom metadata updated",
			"denom", md.Base,
			"display", md.Display,
			"previous_display", existing.Display,
			"units", len(md.DenomUnits),
		)

		if err := ctx.EventManager().EmitTypedEvent(&types.EventSetDenomMetadata{
			Denom:   md.Base,
			Display: md.Display,
		}); err != nil {
			return nil, err
		}
	}

	return &types.MsgSetDenomMetadataResponse{}, nil
}
