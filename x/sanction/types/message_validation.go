package types

import (
	"strings"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	ethcommon "github.com/ethereum/go-ethereum/common"
)

// HasHexAddressPrefix reports whether account looks like it's meant to be an
// EVM-style hex address rather than a bech32 one, based solely on its "0x"/
// "0X" prefix (it says nothing about whether the rest of the string is
// actually valid hex). Shared with keeper.normalizeAddress so the message
// validation here and the keeper's own normalization can't drift apart on
// what counts as "hex-shaped".
func HasHexAddressPrefix(account string) bool {
	return strings.HasPrefix(account, "0x") || strings.HasPrefix(account, "0X")
}

func validateBlacklistMessage(authority string, accounts []string) error {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid authority address (%s)", err)
	}

	if len(accounts) == 0 {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "accounts list cannot be empty")
	}

	for _, account := range accounts {
		if HasHexAddressPrefix(account) {
			if !ethcommon.IsHexAddress(account) {
				return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid hex account address: %s", account)
			}
		} else {
			if _, err := sdk.AccAddressFromBech32(account); err != nil {
				return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid account address: %s", account)
			}
		}
	}

	return nil
}
