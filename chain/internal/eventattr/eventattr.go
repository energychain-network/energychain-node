// Package eventattr collects the event attribute keys that cross module
// boundaries — for example "did", "asset_id", "topic" — so a single
// constant rename does not require touching every keeper.
//
// Modules MAY define their own private attribute keys for purely
// module-local events; only attributes that show up in audit log dumps,
// cross-module routing, or off-chain ETL pipelines belong here.
package eventattr

const (
	// AttrDID is the bech32 / did: identifier of the actor that triggered
	// an event. Used by every module that emits audit-bound events.
	AttrDID = "did"

	// AttrSubject is the *target* of an action when it differs from the
	// actor (e.g. "registry revoked credential X for SUBJECT").
	AttrSubject = "subject"

	// AttrAssetID names the on-chain asset row affected by the event:
	// EAC certificate ID, carbon credit ID, RWA share class, etc.
	AttrAssetID = "asset_id"

	// AttrTopic is the oracle-topic identifier for events emitted by
	// x/oracle.
	AttrTopic = "topic"

	// AttrSeverity tags x/audit events with one of {info, warn, critical}
	// so subscriber tools can route alerts.
	AttrSeverity = "severity"

	// AttrSchemaURI references the off-chain schema document that
	// describes the structured payload carried in `data`.
	AttrSchemaURI = "schema_uri"

	// AttrAmount is the numeric quantity (in the asset's native unit) the
	// event involves. Stored as a base-10 string so very large balances
	// survive JSON encoders that lose precision past 2^53.
	AttrAmount = "amount"

	// AttrReason is an opaque human-readable rationale (revocations,
	// disputes, freezes). Bounded to 256 chars by the audit module.
	AttrReason = "reason"

	// AttrTxHash optionally pins an event to its parent transaction;
	// useful when the event is emitted by an end-blocker that does not
	// otherwise carry tx context.
	AttrTxHash = "tx_hash"
)

// Severity values used with AttrSeverity. Constants — not an enum — so
// downstream JSON serializers don't have to translate.
const (
	SeverityInfo     = "info"
	SeverityWarn     = "warn"
	SeverityCritical = "critical"
)
