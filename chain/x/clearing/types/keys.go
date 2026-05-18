package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "clearing"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	MemberCollectionPrefix = collections.NewPrefix(0x10)
	MemberIDSeqPrefix      = collections.NewPrefix(0x11)
	// Address → member_id reverse index. Prevents two member
	// registrations against the same Bech32 address; queried by
	// SubmitObligation to find the canonical id given a signer.
	MemberByAddrPrefix = collections.NewPrefix(0x12)

	CycleCollectionPrefix = collections.NewPrefix(0x20)
	CycleIDSeqPrefix      = collections.NewPrefix(0x21)

	ObligationCollectionPrefix = collections.NewPrefix(0x30)
	ObligationIDSeqPrefix      = collections.NewPrefix(0x31)
	// Iterate (cycle_id) → obligation_id for Settle's netting
	// loop. Cycle iteration is bounded by params.MaxObligationsPerCycle.
	ObligationByCyclePrefix = collections.NewPrefix(0x32)

	// NetPositions are computed at SettleCycle and persisted
	// (cycle_id, member_id, denom) → NetPosition. Kept on
	// chain so post-settle observers can attribute who paid
	// what without rerunning the netting algo.
	NetPositionCollectionPrefix = collections.NewPrefix(0x40)

	DefaultEventCollectionPrefix = collections.NewPrefix(0x50)

	// Margin: (member_id, denom) → uint64. Stored separately
	// from the Member message to keep the message size bounded
	// when there are many denoms.
	MarginCollectionPrefix = collections.NewPrefix(0x60)

	// Reservation: (member_id, denom) → uint64. The sum of
	// outstanding outbound obligations across all non-terminal
	// cycles for this member-denom. Incremented at submit,
	// decremented at settle / cancel. Used to refuse
	// WithdrawMargin that would drop balance below the gross
	// pending outflow — closes the underwriting-leakage hole
	// where a member could submit then withdraw and default.
	ReservationCollectionPrefix = collections.NewPrefix(0x61)

	// DefaultFund: denom → uint64 (mutualized bucket, drawn
	// after the defaulter's own margin in the waterfall).
	DefaultFundCollectionPrefix = collections.NewPrefix(0x70)
)
