package types

import errorsmod "cosmossdk.io/errors"

var (
	ErrInvalidAddress = errorsmod.Register(ModuleName, 2, "invalid bech32 address")
	ErrUnauthorized   = errorsmod.Register(ModuleName, 3, "unauthorized")
	ErrNotFound       = errorsmod.Register(ModuleName, 4, "not found")
	ErrInvalidField   = errorsmod.Register(ModuleName, 5, "invalid field")
	ErrLimitExceeded  = errorsmod.Register(ModuleName, 6, "resource limit exceeded")
	ErrScheduleState  = errorsmod.Register(ModuleName, 7, "invalid schedule status")
	ErrStreamState    = errorsmod.Register(ModuleName, 8, "invalid stream status")
	ErrOverflow       = errorsmod.Register(ModuleName, 9, "arithmetic overflow")
	ErrSettlement     = errorsmod.Register(ModuleName, 10, "settlement error")
	ErrCompliance     = errorsmod.Register(ModuleName, 11, "compliance check failed")
	ErrActionFailed   = errorsmod.Register(ModuleName, 12, "scheduled action failed")
	ErrNothingVested  = errorsmod.Register(ModuleName, 13, "nothing withdrawable")
)
