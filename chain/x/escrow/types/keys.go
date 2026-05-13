package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "escrow"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	EscrowCollectionPrefix = collections.NewPrefix(0x01) // id -> Escrow
	EscrowIDSeqPrefix      = collections.NewPrefix(0x02)

	// EscrowByDepositorPrefix / EscrowByBeneficiaryPrefix are
	// (account, escrow_id) keysets so per-party listings are
	// O(party_rows) without scanning the full table.
	EscrowByDepositorPrefix   = collections.NewPrefix(0x03)
	EscrowByBeneficiaryPrefix = collections.NewPrefix(0x04)

	// ApprovalCollectionPrefix is keyed (escrow_id, signer) so a
	// per-escrow approval walk is bounded by the committee size.
	ApprovalCollectionPrefix = collections.NewPrefix(0x05) // (escrow_id, signer) -> Approval
)
