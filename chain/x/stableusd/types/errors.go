package types

import errorsmod "cosmossdk.io/errors"

var (
	ErrInvalidAddress    = errorsmod.Register(ModuleName, 2, "invalid bech32 address")
	ErrUnauthorized      = errorsmod.Register(ModuleName, 3, "unauthorized")
	ErrNotFound          = errorsmod.Register(ModuleName, 4, "not found")
	ErrAlreadyExists     = errorsmod.Register(ModuleName, 5, "already exists")
	ErrInvalidField      = errorsmod.Register(ModuleName, 6, "invalid field")
	ErrLimitExceeded     = errorsmod.Register(ModuleName, 7, "resource limit exceeded")
	ErrInsufficientFunds = errorsmod.Register(ModuleName, 8, "insufficient balance")
	ErrInsufficientAllow = errorsmod.Register(ModuleName, 9, "insufficient allowance")
	ErrDenomState        = errorsmod.Register(ModuleName, 10, "invalid denom status")
	ErrCompliance        = errorsmod.Register(ModuleName, 11, "compliance denied")
	ErrReserve           = errorsmod.Register(ModuleName, 12, "reserve coverage insufficient")
	ErrOverflow          = errorsmod.Register(ModuleName, 13, "arithmetic overflow")
	ErrRedemptionState   = errorsmod.Register(ModuleName, 14, "invalid redemption status")
	ErrFrozen            = errorsmod.Register(ModuleName, 15, "account frozen or blacklisted")
)
