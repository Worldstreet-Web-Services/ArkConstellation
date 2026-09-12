package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	"github.com/MANTRA-Chain/mantrachain/v8/x/sanction/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	ethcommon "github.com/ethereum/go-ethereum/common"
)

func normalizeAddress(account string) (string, error) {
	if types.HasHexAddressPrefix(account) {
		if !ethcommon.IsHexAddress(account) {
			return "", errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid hex address: %s", account)
		}
		return sdk.AccAddress(ethcommon.HexToAddress(account).Bytes()).String(), nil
	}
	addr, err := sdk.AccAddressFromBech32(account)
	if err != nil {
		return "", errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid account address: %s (%s)", account, err)
	}
	return addr.String(), nil
}

// AddToBlacklist adds each of the given accounts to the blacklist, returning
// an error if any of them is already blacklisted.
func (k Keeper) AddToBlacklist(ctx context.Context, accounts []string) error {
	for _, account := range accounts {
		normAddr, err := normalizeAddress(account)
		if err != nil {
			return err
		}

		hasAccount, err := k.BlacklistAccounts.Has(ctx, normAddr)
		if err != nil {
			return err
		}
		if hasAccount {
			return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "account %s has already been blacklisted", account)
		}

		if err := k.BlacklistAccounts.Set(ctx, normAddr); err != nil {
			return err
		}
	}

	return nil
}

// RemoveFromBlacklist removes each of the given accounts from the blacklist,
// returning an error if any of them is not currently blacklisted.
func (k Keeper) RemoveFromBlacklist(ctx context.Context, accounts []string) error {
	for _, account := range accounts {
		normAddr, err := normalizeAddress(account)
		if err != nil {
			return err
		}

		hasAccount, err := k.BlacklistAccounts.Has(ctx, normAddr)
		if err != nil {
			return err
		}
		if !hasAccount {
			return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "blacklist account %s is not blacklisted", account)
		}

		if err := k.BlacklistAccounts.Remove(ctx, normAddr); err != nil {
			return err
		}
	}

	return nil
}
