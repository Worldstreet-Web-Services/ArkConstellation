package types

import sdkerrors "cosmossdk.io/errors"

var (
	ErrInvalidAuthority = sdkerrors.Register(ModuleName, 2, "invalid authority")
	ErrEmptyMetadata    = sdkerrors.Register(ModuleName, 3, "no metadata provided")
	ErrDenomNotFound    = sdkerrors.Register(ModuleName, 4, "denomination does not exist on this chain")
	ErrInvalidMetadata  = sdkerrors.Register(ModuleName, 5, "invalid denom metadata")
)
