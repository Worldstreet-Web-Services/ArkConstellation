package types

import (
	sdkerrors "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var _ sdk.Msg = &MsgSetDenomMetadata{}

// ValidateBasic performs stateless validation.
//
// banktypes.Metadata.Validate() does the heavy lifting, and it is stricter than
// it first appears — it already rejects both halves of the shape that causes
// decimals() = 0:
//
//   - the first denom unit must be the base denom at exponent 0, and
//   - a unit named by `display` must exist.
//
// The metadata a chain synthesises on ICS-20 receipt violates both: its single
// unit is "amantra" while the base is the ibc/<hash>, and its display names a
// unit that is not there. So the live metadata on this chain would not pass
// bank's own validation — it exists only because the transfer module writes it
// directly, bypassing Validate().
//
// An earlier revision of this file added a bespoke display-resolution check.
// It was removed once bank's own rules were tested rather than assumed: it
// duplicated them and produced a worse error message.
func (msg *MsgSetDenomMetadata) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return sdkerrors.Wrapf(ErrInvalidAuthority, "invalid authority address %q: %s", msg.Authority, err)
	}

	if len(msg.Metadata) == 0 {
		return ErrEmptyMetadata
	}

	seen := make(map[string]struct{}, len(msg.Metadata))
	for i, md := range msg.Metadata {
		if err := md.Validate(); err != nil {
			return sdkerrors.Wrapf(ErrInvalidMetadata, "metadata[%d] (%s): %s", i, md.Base, err)
		}

		// Bank validates each entry in isolation and cannot see a repeat. Two
		// entries for one denom would both be written, in order, making the
		// result depend on message ordering rather than on what was voted for.
		if _, dup := seen[md.Base]; dup {
			return sdkerrors.Wrapf(ErrInvalidMetadata,
				"metadata[%d]: duplicate entry for denom %q in the same message", i, md.Base)
		}
		seen[md.Base] = struct{}{}
	}

	return nil
}
