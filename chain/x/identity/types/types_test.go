package types_test

import (
	"strings"
	"testing"

	"energychain/x/identity/types"
)

func TestValidatePolicyID(t *testing.T) {
	valid := []string{"pol", "polx", "pol-1", "kyc.eu_2", "a", "p" + strings.Repeat("0", 63)}
	for _, id := range valid {
		if err := types.ValidatePolicyID(id); err != nil {
			t.Errorf("ValidatePolicyID(%q) = %v, want nil", id, err)
		}
	}
	invalid := []string{"", "Pol", "1pol", "-pol", "pol id", "pol!", "p" + strings.Repeat("0", 64)}
	for _, id := range invalid {
		if err := types.ValidatePolicyID(id); err == nil {
			t.Errorf("ValidatePolicyID(%q) = nil, want error", id)
		}
	}
}

func TestValidateJurisdiction(t *testing.T) {
	for _, j := range []string{"US", "HK", "sg-1", "EU.27", "x"} {
		if err := types.ValidateJurisdiction(j); err != nil {
			t.Errorf("ValidateJurisdiction(%q) = %v, want nil", j, err)
		}
	}
	for _, j := range []string{"", "US/CA", "this_is_way_too_long_to_be_a_valid_jurisdiction_code_xx"} {
		if err := types.ValidateJurisdiction(j); err == nil {
			t.Errorf("ValidateJurisdiction(%q) = nil, want error", j)
		}
	}
	// Optional accepts empty, rejects malformed.
	if err := types.ValidateOptionalJurisdiction(""); err != nil {
		t.Errorf("ValidateOptionalJurisdiction(empty) = %v, want nil", err)
	}
	if err := types.ValidateOptionalJurisdiction("US/CA"); err == nil {
		t.Error("ValidateOptionalJurisdiction(US/CA) = nil, want error")
	}
}

func TestValidateDID(t *testing.T) {
	for _, d := range []string{"", "did:example:123", "did:web:abc.com"} {
		if err := types.ValidateDID(d); err != nil {
			t.Errorf("ValidateDID(%q) = %v, want nil", d, err)
		}
	}
	for _, d := range []string{"abc", "uid:1", "did", strings.Repeat("a", types.DIDMaxLen+1)} {
		if err := types.ValidateDID(d); err == nil {
			t.Errorf("ValidateDID(%q) = nil, want error", d)
		}
	}
}

func TestValidateDisplayName(t *testing.T) {
	for _, n := range []string{"", "Alice", "Org (HK) Ltd.", "node-1/eu"} {
		if err := types.ValidateDisplayName(n); err != nil {
			t.Errorf("ValidateDisplayName(%q) = %v, want nil", n, err)
		}
	}
	for _, n := range []string{"bad\tname", "emoji😀", strings.Repeat("a", types.DisplayNameMaxLen+1)} {
		if err := types.ValidateDisplayName(n); err == nil {
			t.Errorf("ValidateDisplayName(%q) = nil, want error", n)
		}
	}
}

func TestValidateModuleTag(t *testing.T) {
	for _, m := range []string{"market", "rwa.token", "x-1"} {
		if err := types.ValidateModuleTag(m); err != nil {
			t.Errorf("ValidateModuleTag(%q) = %v, want nil", m, err)
		}
	}
	for _, m := range []string{"", "Market", "1mod", "mod tag"} {
		if err := types.ValidateModuleTag(m); err == nil {
			t.Errorf("ValidateModuleTag(%q) = nil, want error", m)
		}
	}
}

func TestAccountStatusValid(t *testing.T) {
	if !types.AccountStatusValid(types.AccountStatus_ACCOUNT_STATUS_ACTIVE) ||
		!types.AccountStatusValid(types.AccountStatus_ACCOUNT_STATUS_FROZEN) {
		t.Error("expected ACTIVE and FROZEN to be valid")
	}
	if types.AccountStatusValid(types.AccountStatus_ACCOUNT_STATUS_UNSPECIFIED) {
		t.Error("UNSPECIFIED should be invalid")
	}
}

func TestDefaultParamsValidate(t *testing.T) {
	if err := types.DefaultParams().Validate(); err != nil {
		t.Fatalf("DefaultParams().Validate() = %v, want nil", err)
	}
	if !types.DefaultParams().RequireSanctionsClear {
		t.Error("DefaultParams should require sanctions clear")
	}
}

func TestParamsValidateBounds(t *testing.T) {
	cases := map[string]func(*types.Params){
		"zero registrars":      func(p *types.Params) { p.MaxRegistrars = 0 },
		"too many registrars":  func(p *types.Params) { p.MaxRegistrars = types.HardMaxRegistrars + 1 },
		"zero policies":        func(p *types.Params) { p.MaxPolicies = 0 },
		"too many policies":    func(p *types.Params) { p.MaxPolicies = types.HardMaxPolicies + 1 },
		"zero jurisdictions":   func(p *types.Params) { p.MaxJurisdictionsPerPolicy = 0 },
		"zero audit detail":    func(p *types.Params) { p.AuditMaxDetailLen = 0 },
		"too long audit detail": func(p *types.Params) { p.AuditMaxDetailLen = types.HardAuditMaxDetailLen + 1 },
	}
	for name, mutate := range cases {
		p := types.DefaultParams()
		mutate(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: Validate() = nil, want error", name)
		}
	}
}

func TestValidatePolicyShape(t *testing.T) {
	if err := types.ValidatePolicyShape("kyc", "desc", []string{"US", "HK"}, []string{"KP"}, 64); err != nil {
		t.Fatalf("valid policy shape: %v", err)
	}
	// bad id
	if err := types.ValidatePolicyShape("BAD", "", nil, nil, 64); err == nil {
		t.Error("expected error for bad policy id")
	}
	// allowed list over cap
	if err := types.ValidatePolicyShape("kyc", "", []string{"US", "HK"}, nil, 1); err == nil {
		t.Error("expected error for allowed list over cap")
	}
	// denied list over cap
	if err := types.ValidatePolicyShape("kyc", "", nil, []string{"US", "HK"}, 1); err == nil {
		t.Error("expected error for denied list over cap")
	}
	// malformed jurisdiction
	if err := types.ValidatePolicyShape("kyc", "", []string{"US/CA"}, nil, 64); err == nil {
		t.Error("expected error for malformed allowed jurisdiction")
	}
	// over-long description
	if err := types.ValidatePolicyShape("kyc", strings.Repeat("d", types.DescriptionMaxLen+1), nil, nil, 64); err == nil {
		t.Error("expected error for over-long description")
	}
}
