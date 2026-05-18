package types

import (
	"fmt"
	"math"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Default / hard ceilings. DEFAULTs are what the chain ships
// with; HARDs are governance-proposal ceilings enforced by
// Params.Validate.
const (
	DefaultMaxProviders                uint32 = 4096
	HardMaxProviders                   uint32 = 65_536
	DefaultMaxInfractionsPerProvider   uint32 = 1024
	HardMaxInfractionsPerProvider      uint32 = 65_536
	DefaultMaxJurisdictionsPerProvider uint32 = 64
	HardMaxJurisdictionsPerProvider    uint32 = 256
	DefaultMinBond                     uint64 = 1
	HardMaxMinBond                     uint64 = 1_000_000_000_000_000
	DefaultBondDenom                          = "ustake"
	DefaultSlashBpsMisreport           uint32 = 1_000 // 10%
	DefaultSlashBpsStale               uint32 = 100   // 1%
	DefaultSlashBpsMissingSig          uint32 = 200   // 2%
	DefaultSlashBpsEquivocation        uint32 = 10_000 // 100%
	DefaultSlashBpsUnreachable         uint32 = 50    // 0.5%
	DefaultSlashBpsDisputeRuling       uint32 = 5_000 // 50%
	DefaultSlashBpsOther               uint32 = 500   // 5%
	HardMaxSlashBps                    uint32 = 10_000
	DefaultAutoJailThreshold           uint32 = 3
	HardMaxAutoJailThreshold           uint32 = 1024
	DefaultAutoJailSeconds             int64  = 7 * 24 * 3600
	HardMaxAutoJailSeconds             int64  = 90 * 24 * 3600
	DefaultAutoBanThreshold            uint32 = 10
	HardMaxAutoBanThreshold            uint32 = 4096
	DefaultUnbondCooldownSeconds       int64  = 14 * 24 * 3600
	HardMaxUnbondCooldownSeconds       int64  = 180 * 24 * 3600
	DefaultMemoMaxLen                  uint32 = 256
	HardMemoMaxLen                     uint32 = 1024
	DefaultReasonMaxLen                uint32 = 512
	HardReasonMaxLen                   uint32 = 2048
	DefaultURIMaxLen                   uint32 = 512
	HardURIMaxLen                      uint32 = 4096
	DefaultMaxProcessingsPerBlock      uint32 = 128
	HardMaxProcessingsPerBlock         uint32 = 4096

	DenomMaxLen      = 64
	NameMaxLen       = 128
	DIDMaxLen        = 256
	HashHexLen       = 64
	JurisCodeLen     = 2
)

// ---- Numerical safety --------------------------------------------------

func SafeAdd(a, b uint64) (uint64, error) {
	if a > math.MaxUint64-b {
		return 0, fmt.Errorf("uint64 overflow: %d + %d", a, b)
	}
	return a + b, nil
}

func SafeSub(a, b uint64) (uint64, error) {
	if a < b {
		return 0, fmt.Errorf("uint64 underflow: %d - %d", a, b)
	}
	return a - b, nil
}

// SlashAmount mirrors x/dispute's helper: floor-division
// against basis points so slashed + remainder always sum to
// the original bond.
func SlashAmount(bond uint64, bps uint32) (slashed, remainder uint64) {
	if bps == 0 || bond == 0 {
		return 0, bond
	}
	if bps >= 10_000 {
		return bond, 0
	}
	slashed = bond * uint64(bps) / 10_000
	remainder = bond - slashed
	return
}

// ---- Validators --------------------------------------------------------

func ValidateAddr(field, addr string) error {
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return fmt.Errorf("%s: invalid bech32: %w", field, err)
	}
	return nil
}

func ValidateDID(field, did string) error {
	if did == "" {
		return fmt.Errorf("%s: empty", field)
	}
	if len(did) > DIDMaxLen {
		return fmt.Errorf("%s: too long (>%d)", field, DIDMaxLen)
	}
	if !strings.HasPrefix(did, "did:") {
		return fmt.Errorf("%s: must start with 'did:'", field)
	}
	return nil
}

func ValidateJurisdiction(field, j string) error {
	if j == "GLOBAL" {
		return nil
	}
	if len(j) != JurisCodeLen {
		return fmt.Errorf("%s: ISO-3166-1 alpha-2 or 'GLOBAL', got %q", field, j)
	}
	for _, r := range j {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("%s: ISO code must be uppercase A-Z, got %q", field, j)
		}
	}
	return nil
}

func ValidateHash(field, h string) error {
	if h == "" {
		return nil
	}
	if len(h) != HashHexLen {
		return fmt.Errorf("%s: must be sha256 hex (%d chars), got %d", field, HashHexLen, len(h))
	}
	for _, r := range h {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return fmt.Errorf("%s: non-hex char %q", field, r)
		}
	}
	return nil
}

func ValidateDenom(d string) error {
	if d == "" {
		return fmt.Errorf("denom: empty")
	}
	if len(d) > DenomMaxLen {
		return fmt.Errorf("denom: too long (>%d)", DenomMaxLen)
	}
	return nil
}

func ValidateURI(field, u string, max uint32) error {
	if u == "" {
		return nil
	}
	if uint32(len(u)) > max {
		return fmt.Errorf("%s: too long (%d > %d)", field, len(u), max)
	}
	return nil
}

func ValidateNonEmpty(field, s string, max int) error {
	if s == "" {
		return fmt.Errorf("%s: empty", field)
	}
	if len(s) > max {
		return fmt.Errorf("%s: too long (%d > %d)", field, len(s), max)
	}
	return nil
}

func ValidateReason(s string, max uint32) error {
	if uint32(len(s)) > max {
		return fmt.Errorf("reason: too long (%d > %d)", len(s), max)
	}
	return nil
}

// ---- Enum guards -------------------------------------------------------

func ProviderRoleValid(r ProviderRole) bool {
	switch r {
	case ProviderRole_PROVIDER_ROLE_ORACLE,
		ProviderRole_PROVIDER_ROLE_METER,
		ProviderRole_PROVIDER_ROLE_BRIDGE,
		ProviderRole_PROVIDER_ROLE_OTHER:
		return true
	}
	return false
}

func ProviderStatusValid(s ProviderStatus) bool {
	switch s {
	case ProviderStatus_PROVIDER_STATUS_ACTIVE,
		ProviderStatus_PROVIDER_STATUS_JAILED,
		ProviderStatus_PROVIDER_STATUS_BANNED,
		ProviderStatus_PROVIDER_STATUS_UNBONDING,
		ProviderStatus_PROVIDER_STATUS_WITHDRAWN:
		return true
	}
	return false
}

func InfractionKindValid(k InfractionKind) bool {
	switch k {
	case InfractionKind_INFRACTION_KIND_MISREPORT,
		InfractionKind_INFRACTION_KIND_STALE,
		InfractionKind_INFRACTION_KIND_MISSING_SIG,
		InfractionKind_INFRACTION_KIND_EQUIVOCATION,
		InfractionKind_INFRACTION_KIND_UNREACHABLE,
		InfractionKind_INFRACTION_KIND_DISPUTE_RULING,
		InfractionKind_INFRACTION_KIND_OTHER:
		return true
	}
	return false
}

func ProviderIsTerminal(s ProviderStatus) bool {
	return s == ProviderStatus_PROVIDER_STATUS_BANNED ||
		s == ProviderStatus_PROVIDER_STATUS_WITHDRAWN
}

// ProviderIsActiveForData returns true when upstream modules
// (oracle / meter / bridge) should accept new data from this
// provider's signer. UNBONDING is excluded so a withdrawing
// provider cannot keep accepting attestations until withdraw
// settles.
func ProviderIsActiveForData(s ProviderStatus) bool {
	return s == ProviderStatus_PROVIDER_STATUS_ACTIVE
}

// MinBondFor returns the role-specific bond floor, falling
// back to params.min_bond_default if the role override is 0.
func MinBondFor(p Params, role ProviderRole) uint64 {
	switch role {
	case ProviderRole_PROVIDER_ROLE_ORACLE:
		if p.MinBondOracle > 0 {
			return p.MinBondOracle
		}
	case ProviderRole_PROVIDER_ROLE_METER:
		if p.MinBondMeter > 0 {
			return p.MinBondMeter
		}
	case ProviderRole_PROVIDER_ROLE_BRIDGE:
		if p.MinBondBridge > 0 {
			return p.MinBondBridge
		}
	}
	return p.MinBondDefault
}

// DefaultSlashFor maps an infraction kind to its
// chain-default basis-points slash. Callers may override
// per-report; the chain caps to 10000 either way.
func DefaultSlashFor(p Params, k InfractionKind) uint32 {
	switch k {
	case InfractionKind_INFRACTION_KIND_MISREPORT:
		return p.DefaultSlashBpsMisreport
	case InfractionKind_INFRACTION_KIND_STALE:
		return p.DefaultSlashBpsStale
	case InfractionKind_INFRACTION_KIND_MISSING_SIG:
		return p.DefaultSlashBpsMissingSig
	case InfractionKind_INFRACTION_KIND_EQUIVOCATION:
		return p.DefaultSlashBpsEquivocation
	case InfractionKind_INFRACTION_KIND_UNREACHABLE:
		return p.DefaultSlashBpsUnreachable
	case InfractionKind_INFRACTION_KIND_DISPUTE_RULING:
		return p.DefaultSlashBpsDisputeRuling
	default:
		return p.DefaultSlashBpsOther
	}
}
