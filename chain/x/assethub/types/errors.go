package types

import errorsmod "cosmossdk.io/errors"

var (
	ErrInvalidAddress   = errorsmod.Register(ModuleName, 2, "invalid bech32 address")
	ErrUnauthorized     = errorsmod.Register(ModuleName, 3, "unauthorized")
	ErrNotFound         = errorsmod.Register(ModuleName, 4, "not found")
	ErrAlreadyExists    = errorsmod.Register(ModuleName, 5, "already exists")
	ErrInvalidField     = errorsmod.Register(ModuleName, 6, "invalid field")
	ErrLimitExceeded    = errorsmod.Register(ModuleName, 7, "resource limit exceeded")
	ErrInsufficientBond = errorsmod.Register(ModuleName, 8, "insufficient bond")
	ErrProviderState    = errorsmod.Register(ModuleName, 9, "invalid provider state")
	ErrWrongRole        = errorsmod.Register(ModuleName, 10, "provider role not permitted")
	ErrDeviceRevoked    = errorsmod.Register(ModuleName, 11, "device revoked")
	ErrHasDevices       = errorsmod.Register(ModuleName, 12, "provider still owns active devices")
	ErrInvalidReading   = errorsmod.Register(ModuleName, 13, "invalid reading")
	ErrOverflow         = errorsmod.Register(ModuleName, 14, "arithmetic overflow")
)
