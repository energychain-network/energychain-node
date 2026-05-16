package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// StablecoinKeeper is the cross-module hop x/scheduler takes for
// fee-pool funding (debits owner / credits pool), fee consumption
// (debits pool / credits fee_collector), and withdraw (debits pool
// / credits owner). The pool is a deterministic module account
// owned by x/scheduler — see SchedulerFeePool() in keeper.
//
// The defensive reads (IsAccountBlocked / IsDenomPaused) mirror
// the surface x/escrow uses and let us refuse top-ups / withdraws
// that would otherwise launder around stablecoin's per-denom
// freeze pipeline.
type StablecoinKeeper interface {
	HasDenom(ctx sdk.Context, denomID string) bool
	Move(ctx sdk.Context, denomID, from, to string, amount uint64) error
	IsAccountBlocked(ctx sdk.Context, denomID, account string) bool
	IsDenomPaused(ctx sdk.Context, denomID string) bool
}

// MsgRouter is the narrow surface the scheduler needs to actually
// execute a payload at tick-time. The chain-level adapter wraps
// BaseApp.MsgServiceRouter so this module can stay independent
// of the SDK service router internals.
//
// RouteMsg is invoked inside an isolated sdk.Context.CacheContext
// so a failed payload neither corrupts module state nor short-
// circuits the EndBlocker's iteration over the remaining due jobs.
type MsgRouter interface {
	RouteMsg(ctx sdk.Context, msg sdk.Msg) error
}

// AuditKeeper is the optional cross-module audit hook x/scheduler
// uses for create / cancel / pause / resume / exhaust / withdraw
// transitions. Per-tick execution success/failure is recorded as
// events only (kept in state via last_*_time / failure counters)
// to avoid bloating the audit log on high-frequency jobs.
type AuditKeeper interface {
	RecordSchedulerAction(ctx sdk.Context, jobID uint64, action, actor, detail string)
}
