package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	jurCap := int(gs.Params.MaxJurisdictionsPerPolicy)

	accounts := map[string]bool{}
	for _, a := range gs.Accounts {
		if _, err := sdk.AccAddressFromBech32(a.Address); err != nil {
			return fmt.Errorf("genesis: account %q invalid address: %w", a.Address, err)
		}
		if accounts[a.Address] {
			return fmt.Errorf("genesis: duplicate account %q", a.Address)
		}
		accounts[a.Address] = true
		if !AccountStatusValid(a.Status) {
			return fmt.Errorf("genesis: account %q invalid status", a.Address)
		}
		if err := ValidateDID(a.Did); err != nil {
			return fmt.Errorf("genesis: account %q: %w", a.Address, err)
		}
		if err := ValidateOptionalJurisdiction(a.Jurisdiction); err != nil {
			return fmt.Errorf("genesis: account %q: %w", a.Address, err)
		}
		if a.KycExpiresAt < 0 {
			return fmt.Errorf("genesis: account %q negative kyc_expires_at", a.Address)
		}
	}

	registrars := map[string]bool{}
	for _, r := range gs.Registrars {
		if _, err := sdk.AccAddressFromBech32(r.Address); err != nil {
			return fmt.Errorf("genesis: registrar %q invalid address: %w", r.Address, err)
		}
		if registrars[r.Address] {
			return fmt.Errorf("genesis: duplicate registrar %q", r.Address)
		}
		registrars[r.Address] = true
		if err := ValidateDisplayName(r.DisplayName); err != nil {
			return fmt.Errorf("genesis: registrar %q: %w", r.Address, err)
		}
	}

	if uint32(len(gs.Registrars)) > gs.Params.MaxRegistrars {
		return fmt.Errorf("genesis: registrars %d exceed max %d", len(gs.Registrars), gs.Params.MaxRegistrars)
	}

	sanctions := map[string]bool{}
	for _, s := range gs.Sanctions {
		if _, err := sdk.AccAddressFromBech32(s.Address); err != nil {
			return fmt.Errorf("genesis: sanction %q invalid address: %w", s.Address, err)
		}
		if sanctions[s.Address] {
			return fmt.Errorf("genesis: duplicate sanction %q", s.Address)
		}
		sanctions[s.Address] = true
	}

	policies := map[string]bool{}
	for _, p := range gs.Policies {
		if err := ValidatePolicyShape(p.Id, p.Description, p.AllowedJurisdictions, p.DeniedJurisdictions, jurCap); err != nil {
			return fmt.Errorf("genesis: policy %q: %w", p.Id, err)
		}
		if policies[p.Id] {
			return fmt.Errorf("genesis: duplicate policy %q", p.Id)
		}
		policies[p.Id] = true
		if p.Owner != "" {
			if _, err := sdk.AccAddressFromBech32(p.Owner); err != nil {
				return fmt.Errorf("genesis: policy %q invalid owner: %w", p.Id, err)
			}
		}
	}
	if uint32(len(gs.Policies)) > gs.Params.MaxPolicies {
		return fmt.Errorf("genesis: policies %d exceed max %d", len(gs.Policies), gs.Params.MaxPolicies)
	}

	var maxSeq uint64
	auditSeqs := map[uint64]bool{}
	for _, e := range gs.AuditEntries {
		if e.Seq == 0 {
			return fmt.Errorf("genesis: audit entry seq must be > 0")
		}
		if auditSeqs[e.Seq] {
			return fmt.Errorf("genesis: duplicate audit seq %d", e.Seq)
		}
		auditSeqs[e.Seq] = true
		if e.Seq > maxSeq {
			maxSeq = e.Seq
		}
		if err := ValidateModuleTag(e.Module); err != nil {
			return fmt.Errorf("genesis: audit seq %d: %w", e.Seq, err)
		}
		if uint32(len(e.Detail)) > gs.Params.AuditMaxDetailLen {
			return fmt.Errorf("genesis: audit seq %d detail too long", e.Seq)
		}
	}
	if gs.AuditSeq < maxSeq {
		return fmt.Errorf("genesis: audit_seq %d < max entry seq %d", gs.AuditSeq, maxSeq)
	}

	return nil
}
