package types

import errorsmod "cosmossdk.io/errors"

var (
	ErrInvalidAddress  = errorsmod.Register(ModuleName, 2, "invalid bech32 address")
	ErrUnauthorized    = errorsmod.Register(ModuleName, 3, "unauthorized")
	ErrNotFound        = errorsmod.Register(ModuleName, 4, "not found")
	ErrInvalidField    = errorsmod.Register(ModuleName, 5, "invalid field")
	ErrLimitExceeded   = errorsmod.Register(ModuleName, 6, "resource limit exceeded")
	ErrChainState      = errorsmod.Register(ModuleName, 7, "invalid chain status")
	ErrAssetState      = errorsmod.Register(ModuleName, 8, "invalid asset status")
	ErrInboundState    = errorsmod.Register(ModuleName, 9, "invalid inbound status")
	ErrOverflow        = errorsmod.Register(ModuleName, 10, "arithmetic overflow")
	ErrSettlement      = errorsmod.Register(ModuleName, 11, "settlement error")
	ErrCompliance      = errorsmod.Register(ModuleName, 12, "compliance check failed")
	ErrNotAttestor     = errorsmod.Register(ModuleName, 13, "signer is not a registered attestor")
	ErrAlreadyAttested = errorsmod.Register(ModuleName, 14, "attestor already attested")
	ErrPayloadMismatch = errorsmod.Register(ModuleName, 15, "attestation payload mismatch")
	ErrReplay          = errorsmod.Register(ModuleName, 16, "inbound already processed")
	ErrPaused          = errorsmod.Register(ModuleName, 17, "bridge paused")
	ErrDuplicate       = errorsmod.Register(ModuleName, 18, "duplicate entry")
)
