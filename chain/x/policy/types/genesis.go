package types

import "fmt"

// DefaultParams returns the dev/testnet defaults. Production chains
// should review every cap via governance — particularly max_policies
// and max_bindings, which together bound the worst-case state size.
func DefaultParams() Params {
	return Params{
		MaxRulesPerPolicy: DefaultMaxRulesPerPolicy,
		MaxParamValueSize: DefaultMaxParamValueSize,
		MaxPolicies:       DefaultMaxPolicies,
		MaxBindings:       DefaultMaxBindings,
		LogDenials:        true,
		EvaluationLogMax:  DefaultEvaluationLogMax,
	}
}

func (p Params) Validate() error {
	if p.MaxRulesPerPolicy == 0 || p.MaxRulesPerPolicy > MaxRulesPerPolicyUpper {
		return fmt.Errorf("max_rules_per_policy %d outside (0, %d]",
			p.MaxRulesPerPolicy, MaxRulesPerPolicyUpper)
	}
	if p.MaxParamValueSize == 0 || p.MaxParamValueSize > MaxParamValueSizeUpper {
		return fmt.Errorf("max_param_value_size %d outside (0, %d]",
			p.MaxParamValueSize, MaxParamValueSizeUpper)
	}
	if p.MaxPolicies == 0 || p.MaxPolicies > MaxPoliciesUpper {
		return fmt.Errorf("max_policies %d outside (0, %d]",
			p.MaxPolicies, MaxPoliciesUpper)
	}
	if p.MaxBindings == 0 || p.MaxBindings > MaxBindingsUpper {
		return fmt.Errorf("max_bindings %d outside (0, %d]",
			p.MaxBindings, MaxBindingsUpper)
	}
	if p.EvaluationLogMax > EvaluationLogMaxUpper {
		return fmt.Errorf("evaluation_log_max %d exceeds %d",
			p.EvaluationLogMax, EvaluationLogMaxUpper)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:               DefaultParams(),
		Policies:             []Policy{},
		Bindings:             []PolicyBinding{},
		EvaluationLogs:       []EvaluationLog{},
		EvaluationLogCounter: 0,
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("params: %w", err)
	}
	if uint32(len(gs.Policies)) > gs.Params.MaxPolicies {
		return fmt.Errorf("policies count %d exceeds max %d", len(gs.Policies), gs.Params.MaxPolicies)
	}
	if uint32(len(gs.Bindings)) > gs.Params.MaxBindings {
		return fmt.Errorf("bindings count %d exceeds max %d", len(gs.Bindings), gs.Params.MaxBindings)
	}

	policyIDs := make(map[string]bool, len(gs.Policies))
	for i, p := range gs.Policies {
		if err := ValidatePolicyShape(p, gs.Params.MaxParamValueSize, gs.Params.MaxRulesPerPolicy); err != nil {
			return fmt.Errorf("policy %d: %w", i, err)
		}
		if !PolicyStatusValid(p.Status) {
			return fmt.Errorf("policy %d: invalid status", i)
		}
		if policyIDs[p.Id] {
			return fmt.Errorf("policy %d: duplicate id %s", i, p.Id)
		}
		policyIDs[p.Id] = true
	}

	bindingSeen := make(map[string]bool, len(gs.Bindings))
	for i, b := range gs.Bindings {
		if err := ValidateAssetClass(b.AssetClass); err != nil {
			return fmt.Errorf("binding %d: %w", i, err)
		}
		if err := ValidateAssetID(b.AssetId); err != nil {
			return fmt.Errorf("binding %d: %w", i, err)
		}
		if !policyIDs[b.PolicyId] {
			return fmt.Errorf("binding %d: references unknown policy %s", i, b.PolicyId)
		}
		key := b.AssetClass + "|" + b.AssetId
		if bindingSeen[key] {
			return fmt.Errorf("binding %d: duplicate (asset_class, asset_id) %s", i, key)
		}
		bindingSeen[key] = true
	}

	logIDs := make(map[uint64]bool, len(gs.EvaluationLogs))
	for i, l := range gs.EvaluationLogs {
		if l.Id == 0 {
			return fmt.Errorf("evaluation_log %d: id must be > 0", i)
		}
		if logIDs[l.Id] {
			return fmt.Errorf("evaluation_log %d: duplicate id %d", i, l.Id)
		}
		logIDs[l.Id] = true
		if l.Id > gs.EvaluationLogCounter {
			return fmt.Errorf("evaluation_log %d: id %d exceeds counter %d", i, l.Id, gs.EvaluationLogCounter)
		}
	}
	return nil
}
