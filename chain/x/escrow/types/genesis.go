package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxEscrows:                  DefaultMaxEscrows,
		MaxSignersPerEscrow:         DefaultMaxSignersPerEscrow,
		MaxReleaseHorizonSeconds:    DefaultMaxReleaseHorizonSeconds,
		RequireSanctionsClear:       DefaultRequireSanctionsClear,
	}
}

func (p Params) Validate() error {
	if p.MaxEscrows == 0 || p.MaxEscrows > HardMaxEscrows {
		return fmt.Errorf("max_escrows must be in (0, %d]", HardMaxEscrows)
	}
	if p.MaxSignersPerEscrow == 0 || p.MaxSignersPerEscrow > HardMaxSignersPerEscrow {
		return fmt.Errorf("max_signers_per_escrow must be in (0, %d]", HardMaxSignersPerEscrow)
	}
	if p.MaxReleaseHorizonSeconds <= 0 || p.MaxReleaseHorizonSeconds > HardMaxReleaseHorizonSeconds {
		return fmt.Errorf("max_release_horizon_seconds must be in (0, %d]", HardMaxReleaseHorizonSeconds)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:        DefaultParams(),
		NextEscrowId:  1,
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}
	if uint32(len(gs.Escrows)) > gs.Params.MaxEscrows {
		return fmt.Errorf("genesis: %d escrows exceeds max_escrows %d", len(gs.Escrows), gs.Params.MaxEscrows)
	}

	ids := map[uint64]bool{}
	for _, e := range gs.Escrows {
		if e.Id == 0 {
			return fmt.Errorf("genesis: escrow id must be > 0")
		}
		if ids[e.Id] {
			return fmt.Errorf("genesis: duplicate escrow id %d", e.Id)
		}
		ids[e.Id] = true
		if e.Id >= gs.NextEscrowId {
			return fmt.Errorf("genesis: escrow id %d >= next_escrow_id %d", e.Id, gs.NextEscrowId)
		}
		if err := ValidateAddr("depositor", e.Depositor); err != nil {
			return err
		}
		if err := ValidateAddr("beneficiary", e.Beneficiary); err != nil {
			return err
		}
		if err := ValidateAddr("fallback_addr", e.FallbackAddr); err != nil {
			return err
		}
		if err := ValidateOptionalAddr("arbiter", e.Arbiter); err != nil {
			return err
		}
		if !AssetKindValid(e.Kind) {
			return fmt.Errorf("genesis: escrow %d invalid kind", e.Id)
		}
		if !StatusValid(e.Status) {
			return fmt.Errorf("genesis: escrow %d invalid status", e.Id)
		}
		if e.Amount == 0 {
			return fmt.Errorf("genesis: escrow %d amount must be > 0", e.Id)
		}
		if e.Kind == AssetKind_ASSET_KIND_STABLECOIN {
			if err := ValidateDenom(e.StablecoinDenom); err != nil {
				return fmt.Errorf("genesis: escrow %d: %w", e.Id, err)
			}
			if e.RwaTokenId != 0 {
				return fmt.Errorf("genesis: escrow %d stablecoin escrow MUST NOT set rwa_token_id", e.Id)
			}
		}
		if e.Kind == AssetKind_ASSET_KIND_RWA {
			if e.RwaTokenId == 0 {
				return fmt.Errorf("genesis: escrow %d rwa_token_id must be > 0", e.Id)
			}
			if e.StablecoinDenom != "" {
				return fmt.Errorf("genesis: escrow %d rwa escrow MUST NOT set stablecoin_denom", e.Id)
			}
		}
		if e.ApprovalThreshold == 0 || e.ApprovalThreshold > uint32(len(e.Committee)) {
			return fmt.Errorf("genesis: escrow %d approval_threshold %d invalid for %d signers",
				e.Id, e.ApprovalThreshold, len(e.Committee))
		}
		if uint32(len(e.Committee)) > gs.Params.MaxSignersPerEscrow {
			return fmt.Errorf("genesis: escrow %d has %d signers > max %d", e.Id, len(e.Committee), gs.Params.MaxSignersPerEscrow)
		}
		signerSeen := map[string]bool{}
		for _, s := range e.Committee {
			if err := ValidateAddr("signer", s); err != nil {
				return err
			}
			if signerSeen[s] {
				return fmt.Errorf("genesis: escrow %d duplicate signer %s", e.Id, s)
			}
			signerSeen[s] = true
		}
		if e.ApprovalCountRelease > uint32(len(e.Committee)) {
			return fmt.Errorf("genesis: escrow %d release_count %d > signers %d", e.Id, e.ApprovalCountRelease, len(e.Committee))
		}
		if e.ApprovalCountRefund > uint32(len(e.Committee)) {
			return fmt.Errorf("genesis: escrow %d refund_count %d > signers %d", e.Id, e.ApprovalCountRefund, len(e.Committee))
		}
	}

	approvalSeen := map[string]bool{}
	relCounts := map[uint64]uint32{}
	refCounts := map[uint64]uint32{}
	for _, a := range gs.Approvals {
		if !ids[a.EscrowId] {
			return fmt.Errorf("genesis: approval references unknown escrow %d", a.EscrowId)
		}
		key := fmt.Sprintf("%d|%s", a.EscrowId, a.Signer)
		if approvalSeen[key] {
			return fmt.Errorf("genesis: duplicate approval %s", key)
		}
		approvalSeen[key] = true
		if !IntentValid(a.Intent) {
			return fmt.Errorf("genesis: approval %s invalid intent", key)
		}
		switch a.Intent {
		case Intent_INTENT_RELEASE:
			relCounts[a.EscrowId]++
		case Intent_INTENT_REFUND:
			refCounts[a.EscrowId]++
		}
	}
	for _, e := range gs.Escrows {
		if relCounts[e.Id] != e.ApprovalCountRelease {
			return fmt.Errorf("genesis: escrow %d release_count %d != sum-of-approvals %d", e.Id, e.ApprovalCountRelease, relCounts[e.Id])
		}
		if refCounts[e.Id] != e.ApprovalCountRefund {
			return fmt.Errorf("genesis: escrow %d refund_count %d != sum-of-approvals %d", e.Id, e.ApprovalCountRefund, refCounts[e.Id])
		}
	}

	return nil
}
