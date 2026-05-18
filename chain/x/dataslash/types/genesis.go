package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxProviders:                  DefaultMaxProviders,
		MaxInfractionsPerProvider:     DefaultMaxInfractionsPerProvider,
		MaxJurisdictionsPerProvider:   DefaultMaxJurisdictionsPerProvider,
		MinBondDefault:                DefaultMinBond,
		MinBondOracle:                 0,
		MinBondMeter:                  0,
		MinBondBridge:                 0,
		BondDenomDefault:              DefaultBondDenom,
		DefaultSlashBpsMisreport:      DefaultSlashBpsMisreport,
		DefaultSlashBpsStale:          DefaultSlashBpsStale,
		DefaultSlashBpsMissingSig:     DefaultSlashBpsMissingSig,
		DefaultSlashBpsEquivocation:   DefaultSlashBpsEquivocation,
		DefaultSlashBpsUnreachable:    DefaultSlashBpsUnreachable,
		DefaultSlashBpsDisputeRuling:  DefaultSlashBpsDisputeRuling,
		DefaultSlashBpsOther:          DefaultSlashBpsOther,
		AutoJailThreshold:             DefaultAutoJailThreshold,
		AutoJailSeconds:               DefaultAutoJailSeconds,
		AutoBanThreshold:              DefaultAutoBanThreshold,
		UnbondCooldownSeconds:         DefaultUnbondCooldownSeconds,
		MemoMaxLen:                    DefaultMemoMaxLen,
		ReasonMaxLen:                  DefaultReasonMaxLen,
		UriMaxLen:                     DefaultURIMaxLen,
		MaxProcessingsPerBlock:        DefaultMaxProcessingsPerBlock,
	}
}

func (p Params) Validate() error {
	if p.MaxProviders == 0 || p.MaxProviders > HardMaxProviders {
		return fmt.Errorf("max_providers out of range")
	}
	if p.MaxInfractionsPerProvider == 0 || p.MaxInfractionsPerProvider > HardMaxInfractionsPerProvider {
		return fmt.Errorf("max_infractions_per_provider out of range")
	}
	if p.MaxJurisdictionsPerProvider == 0 || p.MaxJurisdictionsPerProvider > HardMaxJurisdictionsPerProvider {
		return fmt.Errorf("max_jurisdictions_per_provider out of range")
	}
	if p.MinBondDefault == 0 || p.MinBondDefault > HardMaxMinBond {
		return fmt.Errorf("min_bond_default out of range")
	}
	if p.MinBondOracle > HardMaxMinBond {
		return fmt.Errorf("min_bond_oracle > hard cap")
	}
	if p.MinBondMeter > HardMaxMinBond {
		return fmt.Errorf("min_bond_meter > hard cap")
	}
	if p.MinBondBridge > HardMaxMinBond {
		return fmt.Errorf("min_bond_bridge > hard cap")
	}
	if p.BondDenomDefault == "" || len(p.BondDenomDefault) > DenomMaxLen {
		return fmt.Errorf("bond_denom_default invalid")
	}
	for _, v := range []uint32{
		p.DefaultSlashBpsMisreport,
		p.DefaultSlashBpsStale,
		p.DefaultSlashBpsMissingSig,
		p.DefaultSlashBpsEquivocation,
		p.DefaultSlashBpsUnreachable,
		p.DefaultSlashBpsDisputeRuling,
		p.DefaultSlashBpsOther,
	} {
		if v > HardMaxSlashBps {
			return fmt.Errorf("default_slash_bps_* > %d", HardMaxSlashBps)
		}
	}
	if p.AutoJailThreshold == 0 || p.AutoJailThreshold > HardMaxAutoJailThreshold {
		return fmt.Errorf("auto_jail_threshold out of range")
	}
	if p.AutoJailSeconds <= 0 || p.AutoJailSeconds > HardMaxAutoJailSeconds {
		return fmt.Errorf("auto_jail_seconds out of range")
	}
	if p.AutoBanThreshold == 0 || p.AutoBanThreshold > HardMaxAutoBanThreshold {
		return fmt.Errorf("auto_ban_threshold out of range")
	}
	if p.AutoJailThreshold > p.AutoBanThreshold {
		return fmt.Errorf("auto_jail_threshold > auto_ban_threshold (would never ban)")
	}
	if p.UnbondCooldownSeconds < 0 || p.UnbondCooldownSeconds > HardMaxUnbondCooldownSeconds {
		return fmt.Errorf("unbond_cooldown_seconds out of range")
	}
	if p.MemoMaxLen > HardMemoMaxLen {
		return fmt.Errorf("memo_max_len > %d", HardMemoMaxLen)
	}
	if p.ReasonMaxLen > HardReasonMaxLen {
		return fmt.Errorf("reason_max_len > %d", HardReasonMaxLen)
	}
	if p.UriMaxLen == 0 || p.UriMaxLen > HardURIMaxLen {
		return fmt.Errorf("uri_max_len out of range")
	}
	if p.MaxProcessingsPerBlock == 0 || p.MaxProcessingsPerBlock > HardMaxProcessingsPerBlock {
		return fmt.Errorf("max_processings_per_block out of range")
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:            DefaultParams(),
		NextProviderId:    1,
		NextInfractionId:  1,
		NextJailRecordId:  1,
		NextBanRecordId:   1,
	}
}

// Validate runs shape + referential-integrity checks. The
// runtime keeper assumes loaded state already satisfies these.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	if uint32(len(gs.Providers)) > gs.Params.MaxProviders {
		return fmt.Errorf("genesis: %d providers > max %d", len(gs.Providers), gs.Params.MaxProviders)
	}
	provIDs := map[uint64]bool{}
	provDIDs := map[string]bool{}
	provSigners := map[string]bool{}
	for _, p := range gs.Providers {
		if p.Id == 0 {
			return fmt.Errorf("genesis: provider id must be > 0")
		}
		if provIDs[p.Id] {
			return fmt.Errorf("genesis: duplicate provider id %d", p.Id)
		}
		provIDs[p.Id] = true
		if p.Id >= gs.NextProviderId {
			return fmt.Errorf("genesis: provider id %d >= next_provider_id %d", p.Id, gs.NextProviderId)
		}
		if !ProviderStatusValid(p.Status) {
			return fmt.Errorf("genesis: provider %d invalid status", p.Id)
		}
		if !ProviderRoleValid(p.Role) {
			return fmt.Errorf("genesis: provider %d invalid role", p.Id)
		}
		if err := ValidateDID("provider.did", p.Did); err != nil {
			return err
		}
		if provDIDs[p.Did] {
			return fmt.Errorf("genesis: duplicate provider DID %s", p.Did)
		}
		provDIDs[p.Did] = true
		if err := ValidateAddr("provider.signer_address", p.SignerAddress); err != nil {
			return err
		}
		if provSigners[p.SignerAddress] {
			return fmt.Errorf("genesis: duplicate provider signer %s", p.SignerAddress)
		}
		provSigners[p.SignerAddress] = true
		if err := ValidateAddr("provider.bond_owner", p.BondOwner); err != nil {
			return err
		}
		if err := ValidateNonEmpty("provider.name", p.Name, NameMaxLen); err != nil {
			return err
		}
		if err := ValidateDenom(p.BondDenom); err != nil {
			return err
		}
		if uint32(len(p.Jurisdictions)) > gs.Params.MaxJurisdictionsPerProvider {
			return fmt.Errorf("genesis: provider %d too many jurisdictions", p.Id)
		}
		for _, j := range p.Jurisdictions {
			if err := ValidateJurisdiction("provider.jurisdictions", j); err != nil {
				return err
			}
		}
		// Status-specific invariants.
		switch p.Status {
		case ProviderStatus_PROVIDER_STATUS_JAILED:
			if p.JailUntil <= 0 {
				return fmt.Errorf("genesis: provider %d JAILED but jail_until <= 0", p.Id)
			}
		case ProviderStatus_PROVIDER_STATUS_UNBONDING:
			if p.WithdrawAt <= 0 {
				return fmt.Errorf("genesis: provider %d UNBONDING but withdraw_at <= 0", p.Id)
			}
		case ProviderStatus_PROVIDER_STATUS_WITHDRAWN:
			if p.BondAmount != 0 {
				return fmt.Errorf("genesis: provider %d WITHDRAWN but bond_amount != 0", p.Id)
			}
		}
	}

	infIDs := map[uint64]bool{}
	infCount := map[uint64]uint32{}
	for _, inf := range gs.Infractions {
		if inf.Id == 0 {
			return fmt.Errorf("genesis: infraction id must be > 0")
		}
		if infIDs[inf.Id] {
			return fmt.Errorf("genesis: duplicate infraction id %d", inf.Id)
		}
		infIDs[inf.Id] = true
		if inf.Id >= gs.NextInfractionId {
			return fmt.Errorf("genesis: infraction id %d >= next %d", inf.Id, gs.NextInfractionId)
		}
		if !provIDs[inf.ProviderId] {
			return fmt.Errorf("genesis: infraction %d references unknown provider %d", inf.Id, inf.ProviderId)
		}
		if !InfractionKindValid(inf.Kind) {
			return fmt.Errorf("genesis: infraction %d invalid kind", inf.Id)
		}
		if inf.SlashBps > 10_000 {
			return fmt.Errorf("genesis: infraction %d slash_bps > 10000", inf.Id)
		}
		if err := ValidateAddr("infraction.reporter", inf.Reporter); err != nil {
			return err
		}
		if err := ValidateHash("infraction.evidence_hash", inf.EvidenceHash); err != nil {
			return err
		}
		infCount[inf.ProviderId]++
		if infCount[inf.ProviderId] > gs.Params.MaxInfractionsPerProvider {
			return fmt.Errorf("genesis: provider %d infractions > max", inf.ProviderId)
		}
	}

	jrIDs := map[uint64]bool{}
	for _, jr := range gs.JailRecords {
		if jr.Id == 0 {
			return fmt.Errorf("genesis: jail_record id must be > 0")
		}
		if jrIDs[jr.Id] {
			return fmt.Errorf("genesis: duplicate jail_record id %d", jr.Id)
		}
		jrIDs[jr.Id] = true
		if jr.Id >= gs.NextJailRecordId {
			return fmt.Errorf("genesis: jail_record id %d >= next %d", jr.Id, gs.NextJailRecordId)
		}
		if !provIDs[jr.ProviderId] {
			return fmt.Errorf("genesis: jail_record %d references unknown provider %d", jr.Id, jr.ProviderId)
		}
	}

	brIDs := map[uint64]bool{}
	for _, br := range gs.BanRecords {
		if br.Id == 0 {
			return fmt.Errorf("genesis: ban_record id must be > 0")
		}
		if brIDs[br.Id] {
			return fmt.Errorf("genesis: duplicate ban_record id %d", br.Id)
		}
		brIDs[br.Id] = true
		if br.Id >= gs.NextBanRecordId {
			return fmt.Errorf("genesis: ban_record id %d >= next %d", br.Id, gs.NextBanRecordId)
		}
		if !provIDs[br.ProviderId] {
			return fmt.Errorf("genesis: ban_record %d references unknown provider %d", br.Id, br.ProviderId)
		}
	}
	return nil
}
