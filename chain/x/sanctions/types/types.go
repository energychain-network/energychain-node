package types

import (
	"fmt"
	"regexp"
	"strings"
)

// Hard caps and defaults for Params validation.
const (
	ListIDMaxLen          = 64
	ListNameMaxLen        = 128
	ListDescriptionMaxLen = 1_024
	SourceURIMaxLen       = 512
	SourceDigestMaxLen    = 128
	JurisdictionMaxLen    = 8 // ISO-3166 alpha-2 (+ regional codes like EU)
	ProgramMaxLen         = 64
	ReasonMaxLen          = 512
	SourceRefMaxLen       = 256
	SubjectMaxLen         = 256
	ActionMaxLen          = 64
	AssetClassMaxLen      = 32
	AssetIDMaxLen         = 128

	DefaultMaxLists              uint32 = 64
	MaxListsUpper                uint32 = 1_024
	DefaultMaxEntriesPerList     uint32 = 100_000
	MaxEntriesPerListUpper       uint32 = 10_000_000
	DefaultMaxProposalActions    uint32 = 1_000
	MaxProposalActionsUpper      uint32 = 50_000
	DefaultHitsLogMax            uint32 = 4_096
	HitsLogMaxUpper              uint32 = 1_048_576
	DefaultMaxPendingProposals   uint32 = 64
	MaxPendingProposalsUpper     uint32 = 1_024
)

// MaxQueryResults caps any non-paginated walk.
const MaxQueryResults = 1000

// Stable identifier shapes — same alphanumeric / dot / dash /
// underscore set used elsewhere.
var (
	listIDRe       = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	listNameRe     = regexp.MustCompile(`^[A-Za-z0-9 _.\-:/()]{1,128}$`)
	jurisdictionRe = regexp.MustCompile(`^[A-Z]{0,8}$`)
)

func ValidateListID(id string) error {
	if !listIDRe.MatchString(id) {
		return fmt.Errorf("list id %q must match [a-z][a-z0-9._-]{0,63}", id)
	}
	return nil
}

func ValidateListName(name string) error {
	if !listNameRe.MatchString(name) {
		return fmt.Errorf("list name %q out of allowed character set", name)
	}
	return nil
}

func ValidateJurisdiction(code string) error {
	if !jurisdictionRe.MatchString(code) {
		return fmt.Errorf("jurisdiction %q must be 0-8 uppercase letters", code)
	}
	return nil
}

// ValidateSubject only checks non-empty + length; per-kind structural
// shape (bech32, did URI) is left to the caller because the chain
// can't always know what the off-chain feed will send. We deliberately
// avoid bech32 parsing here so a typo'd OFAC entry from a CSV scrape
// does not silently disappear.
func ValidateSubject(subject string) error {
	if subject == "" {
		return fmt.Errorf("subject must be non-empty")
	}
	if len(subject) > SubjectMaxLen {
		return fmt.Errorf("subject too long (max %d)", SubjectMaxLen)
	}
	return nil
}

func ValidateProgram(p string) error {
	if len(p) > ProgramMaxLen {
		return fmt.Errorf("program too long (max %d)", ProgramMaxLen)
	}
	return nil
}

func ValidateReason(r string) error {
	if len(r) > ReasonMaxLen {
		return fmt.Errorf("reason too long (max %d)", ReasonMaxLen)
	}
	return nil
}

func ValidateSourceURI(uri string) error {
	if len(uri) > SourceURIMaxLen {
		return fmt.Errorf("source_uri too long (max %d)", SourceURIMaxLen)
	}
	return nil
}

func ValidateSourceDigest(d string) error {
	if len(d) > SourceDigestMaxLen {
		return fmt.Errorf("source_digest too long (max %d)", SourceDigestMaxLen)
	}
	return nil
}

func ValidateSourceRef(r string) error {
	if len(r) > SourceRefMaxLen {
		return fmt.Errorf("source_ref too long (max %d)", SourceRefMaxLen)
	}
	return nil
}

// ListStatusValid / EntryStatusValid / SubjectKindValid filter out the
// zero (UNSPECIFIED) value. Callers set the desired enum explicitly.
func ListStatusValid(s ListStatus) bool {
	switch s {
	case ListStatus_LIST_STATUS_ACTIVE,
		ListStatus_LIST_STATUS_DEPRECATED,
		ListStatus_LIST_STATUS_ARCHIVED:
		return true
	default:
		return false
	}
}

func EntryStatusValid(s EntryStatus) bool {
	switch s {
	case EntryStatus_ENTRY_STATUS_ACTIVE, EntryStatus_ENTRY_STATUS_REMOVED:
		return true
	default:
		return false
	}
}

func SubjectKindValid(k SubjectKind) bool {
	switch k {
	case SubjectKind_SUBJECT_KIND_ADDRESS, SubjectKind_SUBJECT_KIND_DID:
		return true
	default:
		return false
	}
}

func ProposalStatusValid(s ProposalStatus) bool {
	switch s {
	case ProposalStatus_PROPOSAL_STATUS_PENDING,
		ProposalStatus_PROPOSAL_STATUS_APPLIED,
		ProposalStatus_PROPOSAL_STATUS_REJECTED:
		return true
	default:
		return false
	}
}

func ProposalActionKindValid(k ProposalAction_Kind) bool {
	return k == ProposalAction_KIND_ADD || k == ProposalAction_KIND_REMOVE
}

// CanonicalizeSubject returns the on-chain canonical form of a subject
// per kind. Bech32 addresses are case-insensitive on the wire but
// MUST be stored lowercase so an attacker cannot register
// "cosmos1ABC..." and bypass an IsSanctioned("cosmos1abc...") lookup.
// DID URIs are kind-specific and we do NOT case-fold them; the W3C
// DID Core spec leaves the case-sensitivity rule to each method.
func CanonicalizeSubject(kind SubjectKind, subject string) string {
	if kind == SubjectKind_SUBJECT_KIND_ADDRESS {
		return strings.ToLower(strings.TrimSpace(subject))
	}
	return strings.TrimSpace(subject)
}

// TruncateReason caps a reason string at the configured max length.
// Used by RemoveEntry where reasons accumulate across multiple
// remove cycles ("removed: dup | removed: stale | ...").
func TruncateReason(reason string) string {
	if len(reason) > ReasonMaxLen {
		return reason[:ReasonMaxLen]
	}
	return reason
}
