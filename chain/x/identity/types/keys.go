package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "identity"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// Event types and attribute keys.
const (
	EventTypeRegistrar = "identity_registrar"
	EventTypeAccount   = "identity_account"
	EventTypeSanction  = "identity_sanction"
	EventTypePolicy    = "identity_policy"

	AttrAction  = "action"
	AttrSubject = "subject"
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	AccountPrefix = collections.NewPrefix(0x01) // address -> Account

	RegistrarPrefix = collections.NewPrefix(0x02) // address -> Registrar

	SanctionPrefix = collections.NewPrefix(0x03) // address -> Sanction

	PolicyPrefix = collections.NewPrefix(0x04) // id -> Policy

	AuditPrefix         = collections.NewPrefix(0x05) // seq -> AuditEntry
	AuditByModulePrefix = collections.NewPrefix(0x06) // (module, seq)
	AuditSeqPrefix      = collections.NewPrefix(0x07) // sequence
)
