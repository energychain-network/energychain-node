package types

import errorsmod "cosmossdk.io/errors"

var (
	ErrInvalidAddress = errorsmod.Register(ModuleName, 2, "invalid bech32 address")
	ErrUnauthorized   = errorsmod.Register(ModuleName, 3, "unauthorized")
	ErrNotFound       = errorsmod.Register(ModuleName, 4, "not found")
	ErrAlreadyExists  = errorsmod.Register(ModuleName, 5, "already exists")
	ErrInvalidField   = errorsmod.Register(ModuleName, 6, "invalid field")
	ErrLimitExceeded  = errorsmod.Register(ModuleName, 7, "resource limit exceeded")
	ErrMarketState    = errorsmod.Register(ModuleName, 8, "invalid market status")
	ErrInsufficient   = errorsmod.Register(ModuleName, 9, "insufficient balance")
	ErrCompliance     = errorsmod.Register(ModuleName, 10, "compliance check failed")
	ErrOverflow       = errorsmod.Register(ModuleName, 11, "arithmetic overflow")
	ErrSettlement     = errorsmod.Register(ModuleName, 12, "settlement error")
	ErrSlippage       = errorsmod.Register(ModuleName, 13, "slippage bound not met")
	ErrFloorRegressed = errorsmod.Register(ModuleName, 14, "floor price would decrease")
	ErrInvestState    = errorsmod.Register(ModuleName, 15, "invalid invest status")
	ErrNotMatured     = errorsmod.Register(ModuleName, 16, "invest not matured")
	ErrRewardShort    = errorsmod.Register(ModuleName, 17, "reward pool underfunded")
	ErrZeroOut        = errorsmod.Register(ModuleName, 18, "trade rounds to zero output")
)
