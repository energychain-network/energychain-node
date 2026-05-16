package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "scheduler"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	JobCollectionPrefix = collections.NewPrefix(0x01) // id -> Job
	JobIDSeqPrefix      = collections.NewPrefix(0x02)

	JobByOwnerPrefix = collections.NewPrefix(0x03) // (owner, id)

	// JobByNextRunPrefix is the time-sorted "due" index used by
	// the EndBlocker to walk pending jobs in O(due) instead of
	// O(all_jobs). Only ACTIVE jobs are indexed; status changes
	// (Pause / Cancel / Exhaust) remove the entry, and a Resume
	// reinserts at the new next_run_time.
	JobByNextRunPrefix = collections.NewPrefix(0x04) // (next_run_time, id)
)
