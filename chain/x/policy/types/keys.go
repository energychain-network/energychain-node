package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "policy"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// Persistent-store collection prefixes. One byte per logical collection
// keeps the layout migration-friendly: future schema changes add new
// bytes rather than overload an existing one.
var (
	ParamsCollectionPrefix          = collections.NewPrefix(0x00)
	PolicyCollectionPrefix          = collections.NewPrefix(0x01) // id -> Policy
	BindingCollectionPrefix         = collections.NewPrefix(0x02) // (asset_class, asset_id) -> Binding
	BindingByPolicyPrefix           = collections.NewPrefix(0x03) // (policy_id, asset_class, asset_id)
	EvaluationLogCollectionPrefix   = collections.NewPrefix(0x04) // log_id -> EvaluationLog
	EvaluationLogByPolicyPrefix     = collections.NewPrefix(0x05) // (policy_id, log_id)
	EvaluationLogByAssetClassPrefix = collections.NewPrefix(0x06) // (asset_class, log_id)
	EvaluationLogIDSeqPrefix        = collections.NewPrefix(0x07) // global counter
	// EvaluationLogOldestPrefix is the head cursor of the EvaluationLog
	// ring buffer. Eviction reads + bumps it in O(1) so a denial flood
	// cannot turn the buffer into an O(N) DoS vector.
	EvaluationLogOldestPrefix = collections.NewPrefix(0x08)
)
