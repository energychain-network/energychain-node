package types

import (
	"fmt"
	"regexp"

	"energychain/internal/dsl"
)

// Hard caps and defaults for Params validation.
const (
	PolicyIDMaxLen          = 64
	AssetClassMaxLen        = 32
	AssetIDMaxLen           = 128
	PolicyNameMaxLen        = 128
	PolicyVersionMaxLen     = 32
	PolicyDescriptionMaxLen = 1_024
	ReasonMaxLen            = 256
	DetailMaxLen            = 512

	DefaultMaxRulesPerPolicy uint32 = 32
	MaxRulesPerPolicyUpper   uint32 = 256
	DefaultMaxParamValueSize uint32 = 256
	MaxParamValueSizeUpper   uint32 = 4_096
	DefaultMaxPolicies       uint32 = 1_024
	MaxPoliciesUpper         uint32 = 65_536
	DefaultMaxBindings       uint32 = 65_536
	MaxBindingsUpper         uint32 = 1_048_576
	DefaultEvaluationLogMax  uint32 = 1_024
	EvaluationLogMaxUpper    uint32 = 65_536
)

// MaxQueryResults caps any non-paginated walk so the gRPC layer cannot
// be made to return arbitrarily large result sets.
const MaxQueryResults = 1000

// Stable lexical IDs only — the same alphanumeric / dot / dash /
// underscore set we use in oracle topic ids and meter point ids. Keeps
// the on-chain layout uniform across modules.
var (
	policyIDRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-]{0,63}$`)
	assetClassRe   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	assetIDRe      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-]{0,127}$`)
	policyNameRe   = regexp.MustCompile(`^[A-Za-z0-9 _.\-:/]{1,128}$`)
)

// ValidatePolicyID enforces the canonical id shape.
func ValidatePolicyID(id string) error {
	if !policyIDRe.MatchString(id) {
		return fmt.Errorf("policy id %q must match [A-Za-z0-9][A-Za-z0-9._-]{0,63}", id)
	}
	return nil
}

// ValidateAssetClass enforces the canonical class shape. Asset classes
// are intentionally lowercase + snake to avoid case-folding ambiguity
// at the binding lookup.
func ValidateAssetClass(class string) error {
	if !assetClassRe.MatchString(class) {
		return fmt.Errorf("asset_class %q must be lowercase [a-z][a-z0-9_]{0,31}", class)
	}
	return nil
}

// ValidateAssetID enforces the canonical asset id shape.
func ValidateAssetID(id string) error {
	if !assetIDRe.MatchString(id) {
		return fmt.Errorf("asset_id %q must match [A-Za-z0-9][A-Za-z0-9._-]{0,127}", id)
	}
	return nil
}

// ValidatePolicyName accepts a slightly broader character set than the
// id (spaces, colon, slash) so operator-friendly names like "us-reg-d
// 506c v2" round-trip through validation.
func ValidatePolicyName(name string) error {
	if !policyNameRe.MatchString(name) {
		return fmt.Errorf("policy name %q out of allowed character set", name)
	}
	return nil
}

// ToDSLPolicy converts the on-chain protobuf policy into the runtime
// engine type. Used by both the keeper hot path and the Evaluate query.
//
// rules MUST already have passed ValidatePolicyShape — this function is
// O(rules) and copies the params map verbatim.
func ToDSLPolicy(p Policy) dsl.Policy {
	out := dsl.Policy{Name: p.Name, Rules: make([]dsl.Rule, 0, len(p.Rules))}
	for _, r := range p.Rules {
		params := make(map[string]string, len(r.Params))
		for k, v := range r.Params {
			params[k] = v
		}
		out.Rules = append(out.Rules, dsl.Rule{
			Kind:   dsl.RuleKind(r.Kind),
			Side:   dsl.Side(r.Side),
			Params: params,
		})
	}
	return out
}

// ValidatePolicyShape is the proto-side guard. It checks the per-rule
// max-param-size (enforced against Params at registration time) plus
// the structural shape required by the underlying dsl engine.
func ValidatePolicyShape(p Policy, maxParamSize uint32, maxRules uint32) error {
	if err := ValidatePolicyID(p.Id); err != nil {
		return err
	}
	if err := ValidatePolicyName(p.Name); err != nil {
		return err
	}
	if len(p.Description) > PolicyDescriptionMaxLen {
		return fmt.Errorf("description too long")
	}
	if len(p.Version) > PolicyVersionMaxLen {
		return fmt.Errorf("version too long")
	}
	if uint32(len(p.Rules)) > maxRules {
		return fmt.Errorf("policy %s: %d rules exceeds max %d", p.Id, len(p.Rules), maxRules)
	}
	for i, r := range p.Rules {
		for k, v := range r.Params {
			if uint32(len(k))+uint32(len(v)) > maxParamSize {
				return fmt.Errorf("rule %d param %q exceeds max combined size %d", i, k, maxParamSize)
			}
		}
	}
	// Delegate the per-rule structural shape (required keys per Kind)
	// to the internal/dsl validator so the chain and the runtime share
	// one rule table.
	if err := ToDSLPolicy(p).Validate(); err != nil {
		return fmt.Errorf("dsl: %w", err)
	}
	return nil
}

// PolicyStatusValid returns true for non-default enum values; the
// UNSPECIFIED zero is reserved for "fresh" structs and never stored.
func PolicyStatusValid(s PolicyStatus) bool {
	switch s {
	case PolicyStatus_POLICY_STATUS_ACTIVE,
		PolicyStatus_POLICY_STATUS_DEPRECATED,
		PolicyStatus_POLICY_STATUS_DISABLED:
		return true
	default:
		return false
	}
}
