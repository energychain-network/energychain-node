package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "dispute"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	ArbitratorCollectionPrefix   = collections.NewPrefix(0x10) // id → Arbitrator
	ArbitratorIDSeqPrefix        = collections.NewPrefix(0x11)
	ArbitratorByDIDPrefix        = collections.NewPrefix(0x12) // did → id
	ArbitratorBySignerPrefix     = collections.NewPrefix(0x13) // signer_address → id

	DisputeCollectionPrefix      = collections.NewPrefix(0x20) // id → Dispute
	DisputeIDSeqPrefix           = collections.NewPrefix(0x21)
	// DisputeByStatusPrefix walks active disputes for the
	// end-block sweep that auto-cancels lapsed OPEN disputes
	// and force-finalizes lapsed DELIBERATING ones.
	DisputeByStatusPrefix        = collections.NewPrefix(0x22) // (status, id)
	// DisputeBySubjectPrefix lets upstream modules (oracle /
	// meter / contract) cheaply detect any in-flight dispute
	// about a specific subject_ref.
	DisputeBySubjectPrefix       = collections.NewPrefix(0x23) // (subject_kind:uint32, subject_ref, id)

	TribunalCollectionPrefix     = collections.NewPrefix(0x30) // (dispute_id, arbitrator_id) → TribunalMember
	VoteCollectionPrefix         = collections.NewPrefix(0x40) // (dispute_id, arbitrator_id) → Vote
	EvidenceCollectionPrefix     = collections.NewPrefix(0x50) // (dispute_id, evidence_id) → Evidence
	EvidenceSeqCollectionPrefix  = collections.NewPrefix(0x51) // dispute_id → next_evidence_id
)
