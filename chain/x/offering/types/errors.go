package types

import errorsmod "cosmossdk.io/errors"

var (
	ErrInvalidAddress = errorsmod.Register(ModuleName, 2, "invalid bech32 address")
	ErrUnauthorized   = errorsmod.Register(ModuleName, 3, "unauthorized")
	ErrNotFound       = errorsmod.Register(ModuleName, 4, "not found")
	ErrAlreadyExists  = errorsmod.Register(ModuleName, 5, "already exists")
	ErrInvalidField   = errorsmod.Register(ModuleName, 6, "invalid field")
	ErrLimitExceeded  = errorsmod.Register(ModuleName, 7, "resource limit exceeded")
	ErrOfferingState  = errorsmod.Register(ModuleName, 8, "invalid offering status")
	ErrWindow         = errorsmod.Register(ModuleName, 9, "outside subscription window")
	ErrCap            = errorsmod.Register(ModuleName, 10, "cap violation")
	ErrOverflow       = errorsmod.Register(ModuleName, 11, "arithmetic overflow")
	ErrSettlement     = errorsmod.Register(ModuleName, 12, "settlement error")
	ErrRWA            = errorsmod.Register(ModuleName, 13, "rwa token error")
	ErrTrancheGate    = errorsmod.Register(ModuleName, 14, "tranche release gated on injection")
	ErrNothingClaim   = errorsmod.Register(ModuleName, 15, "nothing to claim")
	ErrTreasuryShort  = errorsmod.Register(ModuleName, 16, "treasury underfunded")
	ErrNotDelinquent  = errorsmod.Register(ModuleName, 17, "issuer not delinquent")
	ErrCompliance     = errorsmod.Register(ModuleName, 18, "compliance denied")
)
