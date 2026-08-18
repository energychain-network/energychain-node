package types

import errorsmod "cosmossdk.io/errors"

var (
	ErrInvalidAddress    = errorsmod.Register(ModuleName, 2, "invalid bech32 address")
	ErrUnauthorized      = errorsmod.Register(ModuleName, 3, "unauthorized")
	ErrNotFound          = errorsmod.Register(ModuleName, 4, "not found")
	ErrAlreadyExists     = errorsmod.Register(ModuleName, 5, "already exists")
	ErrInvalidField      = errorsmod.Register(ModuleName, 6, "invalid field")
	ErrLimitExceeded     = errorsmod.Register(ModuleName, 7, "resource limit exceeded")
	ErrInsufficientFunds = errorsmod.Register(ModuleName, 8, "insufficient transferable balance")
	ErrTokenState        = errorsmod.Register(ModuleName, 9, "invalid token status")
	ErrCompliance        = errorsmod.Register(ModuleName, 10, "compliance denied")
	ErrOverflow          = errorsmod.Register(ModuleName, 11, "arithmetic overflow")
	ErrSnapshotState     = errorsmod.Register(ModuleName, 12, "invalid snapshot")
	ErrDistribution      = errorsmod.Register(ModuleName, 13, "invalid distribution")
	ErrRedemptionState   = errorsmod.Register(ModuleName, 14, "invalid redemption status")
	ErrFrozen            = errorsmod.Register(ModuleName, 15, "account frozen")
	ErrSettlement        = errorsmod.Register(ModuleName, 16, "settlement denom error")
	ErrPoolUnderfunded   = errorsmod.Register(ModuleName, 17, "settlement pool underfunded")
)
