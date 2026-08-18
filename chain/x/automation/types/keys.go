package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "automation"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// StreamEscrow is the single module-derived account that custodies all
// stream deposits across denoms (the x/stableusd ledger is per-denom keyed,
// so commingling here is safe and per-stream accounting lives in state).
const StreamEscrowName = "automation/stream-escrow"

const (
	EventTypeSchedule = "automation_schedule"
	EventTypeTick     = "automation_tick"
	EventTypeStream   = "automation_stream"

	AttrAction     = "action"
	AttrScheduleID = "schedule_id"
	AttrStreamID   = "stream_id"
	AttrCreator    = "creator"
	AttrStatus     = "status"
	AttrNextRun    = "next_run"
	AttrError      = "error"
	AttrAmount     = "amount"
	AttrReceiver   = "receiver"
	AttrSender     = "sender"
)

var (
	ParamsCollectionPrefix  = collections.NewPrefix(0x00)
	SchedulePrefix          = collections.NewPrefix(0x01) // id -> Schedule
	ScheduleByNextRunPrefix = collections.NewPrefix(0x02) // (next_run, id) -> active index
	ScheduleIDSeqPrefix     = collections.NewPrefix(0x03)
	StreamPrefix            = collections.NewPrefix(0x04) // id -> Stream
	StreamByStopPrefix      = collections.NewPrefix(0x05) // (stop_time, id) -> active index
	StreamIDSeqPrefix       = collections.NewPrefix(0x06)
)
