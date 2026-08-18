package types

import errorsmod "cosmossdk.io/errors"

// Sentinel errors for the identity module. Codespace is the module
// name; codes are stable and must never be renumbered.
var (
	ErrInvalidAddress    = errorsmod.Register(ModuleName, 2, "invalid bech32 address")
	ErrUnauthorized      = errorsmod.Register(ModuleName, 3, "unauthorized")
	ErrNotFound          = errorsmod.Register(ModuleName, 4, "not found")
	ErrAlreadyExists     = errorsmod.Register(ModuleName, 5, "already exists")
	ErrInvalidPolicy     = errorsmod.Register(ModuleName, 6, "invalid policy")
	ErrTransferDenied    = errorsmod.Register(ModuleName, 7, "transfer denied by compliance policy")
	ErrSanctioned        = errorsmod.Register(ModuleName, 8, "address is sanctioned")
	ErrLimitExceeded     = errorsmod.Register(ModuleName, 9, "resource limit exceeded")
	ErrInvalidField      = errorsmod.Register(ModuleName, 10, "invalid field")
	ErrAccountFrozen     = errorsmod.Register(ModuleName, 11, "account is frozen")
	ErrKYCRequired       = errorsmod.Register(ModuleName, 12, "kyc clearance required")
	ErrAccreditedReq     = errorsmod.Register(ModuleName, 13, "accredited status required")
	ErrJurisdictionDenied = errorsmod.Register(ModuleName, 14, "jurisdiction not permitted")
	ErrPolicyPaused      = errorsmod.Register(ModuleName, 15, "policy is paused")
)
