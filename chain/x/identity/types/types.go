package types

import (
	"fmt"
	"regexp"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	PolicyIDMaxLen      = 64
	DescriptionMaxLen   = 256
	DIDMaxLen           = 256
	JurisdictionMaxLen  = 32
	ReasonMaxLen        = 512
	ListSourceMaxLen    = 64
	DisplayNameMaxLen   = 128
	ActionMaxLen        = 64
	ModuleTagMaxLen     = 32

	DefaultMaxRegistrars             uint32 = 256
	DefaultMaxPolicies               uint32 = 4_096
	DefaultMaxJurisdictionsPerPolicy uint32 = 64
	DefaultAuditMaxDetailLen         uint32 = 1_024

	HardMaxRegistrars             uint32 = 16_384
	HardMaxPolicies               uint32 = 1_000_000
	HardMaxJurisdictionsPerPolicy uint32 = 1_024
	HardAuditMaxDetailLen         uint32 = 16_384
)

var (
	policyIDRe      = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	jurisdictionRe  = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,32}$`)
	displayNameRe   = regexp.MustCompile(`^[A-Za-z0-9 _.\-:/()]{1,128}$`)
	moduleTagRe     = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,31}$`)
)

func MustBech32(addr string) error {
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return errorsmod.Wrapf(ErrInvalidAddress, "%q: %v", addr, err)
	}
	return nil
}

func ValidatePolicyID(id string) error {
	if !policyIDRe.MatchString(id) {
		return errorsmod.Wrapf(ErrInvalidField, "policy id %q must match [a-z][a-z0-9._-]{0,63}", id)
	}
	return nil
}

func ValidateJurisdiction(s string) error {
	if !jurisdictionRe.MatchString(s) {
		return errorsmod.Wrapf(ErrInvalidField, "jurisdiction %q must match [A-Za-z0-9._-]{1,32}", s)
	}
	return nil
}

func ValidateOptionalJurisdiction(s string) error {
	if s == "" {
		return nil
	}
	return ValidateJurisdiction(s)
}

func ValidateDID(did string) error {
	if did == "" {
		return nil // DID is optional on an Account
	}
	if len(did) > DIDMaxLen {
		return errorsmod.Wrapf(ErrInvalidField, "did too long (max %d)", DIDMaxLen)
	}
	if len(did) < 4 || did[:4] != "did:" {
		return errorsmod.Wrapf(ErrInvalidField, "did %q must start with 'did:'", did)
	}
	return nil
}

func ValidateDisplayName(n string) error {
	if n == "" {
		return nil
	}
	if !displayNameRe.MatchString(n) {
		return errorsmod.Wrapf(ErrInvalidField, "display_name %q out of allowed character set", n)
	}
	return nil
}

func ValidateModuleTag(m string) error {
	if !moduleTagRe.MatchString(m) {
		return errorsmod.Wrapf(ErrInvalidField, "module %q must match [a-z][a-z0-9._-]{0,31}", m)
	}
	return nil
}

func ValidateMaxLen(field, v string, max int) error {
	if len(v) > max {
		return errorsmod.Wrapf(ErrInvalidField, "%s too long (max %d)", field, max)
	}
	return nil
}

func AccountStatusValid(s AccountStatus) bool {
	switch s {
	case AccountStatus_ACCOUNT_STATUS_ACTIVE, AccountStatus_ACCOUNT_STATUS_FROZEN:
		return true
	}
	return false
}

func DefaultParams() Params {
	return Params{
		MaxRegistrars:             DefaultMaxRegistrars,
		MaxPolicies:               DefaultMaxPolicies,
		MaxJurisdictionsPerPolicy: DefaultMaxJurisdictionsPerPolicy,
		RequireSanctionsClear:     true,
		AuditMaxDetailLen:         DefaultAuditMaxDetailLen,
	}
}

func (p Params) Validate() error {
	if p.MaxRegistrars == 0 || p.MaxRegistrars > HardMaxRegistrars {
		return fmt.Errorf("max_registrars must be in (0, %d]", HardMaxRegistrars)
	}
	if p.MaxPolicies == 0 || p.MaxPolicies > HardMaxPolicies {
		return fmt.Errorf("max_policies must be in (0, %d]", HardMaxPolicies)
	}
	if p.MaxJurisdictionsPerPolicy == 0 || p.MaxJurisdictionsPerPolicy > HardMaxJurisdictionsPerPolicy {
		return fmt.Errorf("max_jurisdictions_per_policy must be in (0, %d]", HardMaxJurisdictionsPerPolicy)
	}
	if p.AuditMaxDetailLen == 0 || p.AuditMaxDetailLen > HardAuditMaxDetailLen {
		return fmt.Errorf("audit_max_detail_len must be in (0, %d]", HardAuditMaxDetailLen)
	}
	return nil
}

// ValidatePolicyShape checks the rule fields of a policy independent of
// storage. jurisdictionCap bounds each jurisdiction list.
func ValidatePolicyShape(id, description string, allowed, denied []string, jurisdictionCap int) error {
	if err := ValidatePolicyID(id); err != nil {
		return err
	}
	if err := ValidateMaxLen("description", description, DescriptionMaxLen); err != nil {
		return err
	}
	if len(allowed) > jurisdictionCap {
		return errorsmod.Wrapf(ErrLimitExceeded, "allowed_jurisdictions %d > cap %d", len(allowed), jurisdictionCap)
	}
	if len(denied) > jurisdictionCap {
		return errorsmod.Wrapf(ErrLimitExceeded, "denied_jurisdictions %d > cap %d", len(denied), jurisdictionCap)
	}
	for _, j := range allowed {
		if err := ValidateJurisdiction(j); err != nil {
			return err
		}
	}
	for _, j := range denied {
		if err := ValidateJurisdiction(j); err != nil {
			return err
		}
	}
	return nil
}
