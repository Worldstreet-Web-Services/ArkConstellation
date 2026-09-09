package types

import (
	"fmt"

	sdkerrors "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

var _ sdk.Msg = &MsgSetDenomMetadata{}

// ValidateBasic performs stateless validation.
//
// The interesting check is DisplayResolvesToAUnit. The metadata a chain
// synthesises on ICS-20 receipt is internally inconsistent: `display` names a
// unit that is not in `denom_units`, so any consumer resolving decimals by
// looking up the display unit finds nothing and falls back to 0. That is the
// entire bug this module exists to fix, so a message that would write the same
// inconsistency is rejected rather than silently stored.
func (msg *MsgSetDenomMetadata) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return sdkerrors.Wrapf(ErrInvalidAuthority, "invalid authority address %q: %s", msg.Authority, err)
	}

	if len(msg.Metadata) == 0 {
		return ErrEmptyMetadata
	}

	seen := make(map[string]struct{}, len(msg.Metadata))
	for i, md := range msg.Metadata {
		// Validate() covers base/display presence, unit ordering, and that
		// exactly one unit has exponent 0.
		if err := md.Validate(); err != nil {
			return sdkerrors.Wrapf(ErrInvalidMetadata, "metadata[%d] (%s): %s", i, md.Base, err)
		}

		if _, dup := seen[md.Base]; dup {
			return sdkerrors.Wrapf(ErrInvalidMetadata,
				"metadata[%d]: duplicate entry for denom %q in the same message", i, md.Base)
		}
		seen[md.Base] = struct{}{}

		if err := DisplayResolvesToAUnit(md); err != nil {
			return sdkerrors.Wrapf(ErrInvalidMetadata, "metadata[%d]: %s", i, err)
		}
	}

	return nil
}

// DisplayResolvesToAUnit reports whether `display` names a unit present in
// denom_units. Exported so the keeper can apply the same rule to metadata that
// already exists on chain.
func DisplayResolvesToAUnit(md banktypes.Metadata) error {
	names := make([]string, 0, len(md.DenomUnits))
	for _, u := range md.DenomUnits {
		if u == nil {
			continue
		}
		if u.Denom == md.Display {
			return nil
		}
		names = append(names, u.Denom)
	}
	return fmt.Errorf(
		"denom %q: display %q is not present in denom_units %v; decimals would "+
			"resolve to 0. Add a unit named %q with the correct exponent",
		md.Base, md.Display, names, md.Display)
}
