package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// PolicyKeeper is the slice of x/policy the EAC keeper consults on
// every transfer and retire. The keeper passes the canonical binding
// tuple (asset_class="eac", asset_id=<certificate_id_decimal>);
// x/policy resolves the bound rule set internally.
type PolicyKeeper interface {
	EvaluateTransfer(ctx sdk.Context, assetClass, assetID, sender, receiver string, amount uint64) error
}

// SanctionsKeeper is the defense-in-depth address gate. The EAC
// keeper checks this BEFORE policy evaluation so a sanctioned
// counterparty is rejected even when no policy is bound to the
// certificate.
type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, subject string) bool
}

// OracleKeeper is the bridge-attestation gate. Called from
// BridgeMint to ensure the chain doesn't mint more units than the
// upstream registry has confirmed locked.
//
// Returns the attested locked-units count, the timestamp of the
// latest aggregation, and a boolean indicating whether any
// attestation exists. Implementations MUST NOT mutate state.
type OracleKeeper interface {
	GetAggregatedReserve(ctx sdk.Context, topicID string) (value int64, timestamp int64, ok bool)
}

// AuditKeeper is the optional cross-module hook. EAC writes here on
// retirements and force operations so the audit module can compose
// the chain-of-custody trail.
type AuditKeeper interface {
	RecordEACAction(ctx sdk.Context, certificateID uint64, issuerID, action, actor, beneficiary, detail string)
}

// ERC20Keeper is the optional auto-registration hook into
// Cosmos EVM's x/erc20 module. RegisterIssuer reserves a TokenPair
// entry for the synthetic denom "eac"+issuer_id so EVM tooling can
// discover the issuer's certificates as a single namespace. Failure
// is never fatal to MsgRegisterIssuer — see msg_server.go for the
// non-rollback contract.
type ERC20Keeper interface {
	IsDenomRegistered(ctx sdk.Context, denom string) bool
	CreateNewTokenPair(ctx sdk.Context, denom string) error
}
