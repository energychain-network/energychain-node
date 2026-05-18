package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxSchemas:                  DefaultMaxSchemas,
		MaxVerifiers:                DefaultMaxVerifiers,
		MaxReportsPerSubject:        DefaultMaxReportsPerSubject,
		MaxViewKeyGrants:            DefaultMaxViewKeyGrants,
		MemoMaxLen:                  DefaultMemoMaxLen,
		UriMaxLen:                   DefaultURIMaxLen,
		MaxJurisdictionsPerVerifier: DefaultMaxJurisdictionsPerVerifier,
		MaxStandardsPerVerifier:     DefaultMaxStandardsPerVerifier,
		ReasonMaxLen:                DefaultReasonMaxLen,
		MaxGrantTtlSeconds:          DefaultMaxGrantTTLSeconds,
		MaxGrantsPerBlockSweep:      DefaultMaxGrantsPerBlockSweep,
	}
}

func (p Params) Validate() error {
	if p.MaxSchemas == 0 || p.MaxSchemas > HardMaxSchemas {
		return fmt.Errorf("max_schemas out of range")
	}
	if p.MaxVerifiers == 0 || p.MaxVerifiers > HardMaxVerifiers {
		return fmt.Errorf("max_verifiers out of range")
	}
	if p.MaxReportsPerSubject == 0 || p.MaxReportsPerSubject > HardMaxReportsPerSubject {
		return fmt.Errorf("max_reports_per_subject out of range")
	}
	if p.MaxViewKeyGrants == 0 || p.MaxViewKeyGrants > HardMaxViewKeyGrants {
		return fmt.Errorf("max_view_key_grants out of range")
	}
	if p.MemoMaxLen > HardMemoMaxLen {
		return fmt.Errorf("memo_max_len > %d", HardMemoMaxLen)
	}
	if p.UriMaxLen == 0 || p.UriMaxLen > HardURIMaxLen {
		return fmt.Errorf("uri_max_len out of range")
	}
	if p.MaxJurisdictionsPerVerifier == 0 || p.MaxJurisdictionsPerVerifier > HardMaxJurisdictionsPerVerifier {
		return fmt.Errorf("max_jurisdictions_per_verifier out of range")
	}
	if p.MaxStandardsPerVerifier == 0 || p.MaxStandardsPerVerifier > HardMaxStandardsPerVerifier {
		return fmt.Errorf("max_standards_per_verifier out of range")
	}
	if p.ReasonMaxLen > HardReasonMaxLen {
		return fmt.Errorf("reason_max_len > %d", HardReasonMaxLen)
	}
	if p.MaxGrantTtlSeconds <= 0 || p.MaxGrantTtlSeconds > HardMaxGrantTTLSeconds {
		return fmt.Errorf("max_grant_ttl_seconds out of range")
	}
	if p.MaxGrantsPerBlockSweep == 0 || p.MaxGrantsPerBlockSweep > HardMaxGrantsPerBlockSweep {
		return fmt.Errorf("max_grants_per_block_sweep out of range")
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:         DefaultParams(),
		NextSchemaId:   1,
		NextVerifierId: 1,
		NextReportId:   1,
		NextGrantId:    1,
	}
}

// Validate runs both shape checks (max counts, uniqueness)
// and referential integrity checks (every report points to a
// known schema and a known verifier (if attested); every grant
// targets a known schema/report etc.). This is the only place
// where genesis-time invariants live; the runtime keeper
// assumes the state it loaded already satisfies them.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	if uint32(len(gs.Schemas)) > gs.Params.MaxSchemas {
		return fmt.Errorf("genesis: %d schemas > max %d", len(gs.Schemas), gs.Params.MaxSchemas)
	}
	schemaIDs := map[uint64]bool{}
	for _, s := range gs.Schemas {
		if s.Id == 0 {
			return fmt.Errorf("genesis: schema id must be > 0")
		}
		if schemaIDs[s.Id] {
			return fmt.Errorf("genesis: duplicate schema id %d", s.Id)
		}
		schemaIDs[s.Id] = true
		if s.Id >= gs.NextSchemaId {
			return fmt.Errorf("genesis: schema id %d >= next_schema_id %d", s.Id, gs.NextSchemaId)
		}
		if !SchemaStatusValid(s.Status) {
			return fmt.Errorf("genesis: schema %d invalid status", s.Id)
		}
		if !AssetClassValid(s.AssetClass) {
			return fmt.Errorf("genesis: schema %d invalid asset_class", s.Id)
		}
		if !TimeWindowValid(s.TimeWindow) {
			return fmt.Errorf("genesis: schema %d invalid time_window", s.Id)
		}
		if !ReportFormatValid(s.OutputFormat) {
			return fmt.Errorf("genesis: schema %d invalid output_format", s.Id)
		}
		if err := ValidateNonEmpty("schema.name", s.Name, NameMaxLen); err != nil {
			return err
		}
		if err := ValidateNonEmpty("schema.version", s.Version, VersionMaxLen); err != nil {
			return err
		}
		if err := ValidateJurisdiction("schema.jurisdiction", s.Jurisdiction); err != nil {
			return err
		}
		if err := ValidateURI("schema.schema_uri", s.SchemaUri, gs.Params.UriMaxLen); err != nil {
			return err
		}
		if err := ValidateHash("schema.schema_hash", s.SchemaHash); err != nil {
			return err
		}
	}

	if uint32(len(gs.Verifiers)) > gs.Params.MaxVerifiers {
		return fmt.Errorf("genesis: %d verifiers > max %d", len(gs.Verifiers), gs.Params.MaxVerifiers)
	}
	verifierIDs := map[uint64]bool{}
	verifierDIDs := map[string]bool{}
	for _, v := range gs.Verifiers {
		if v.Id == 0 {
			return fmt.Errorf("genesis: verifier id must be > 0")
		}
		if verifierIDs[v.Id] {
			return fmt.Errorf("genesis: duplicate verifier id %d", v.Id)
		}
		verifierIDs[v.Id] = true
		if v.Id >= gs.NextVerifierId {
			return fmt.Errorf("genesis: verifier id %d >= next_verifier_id %d", v.Id, gs.NextVerifierId)
		}
		if !VerifierStatusValid(v.Status) {
			return fmt.Errorf("genesis: verifier %d invalid status", v.Id)
		}
		if err := ValidateDID("verifier.did", v.Did); err != nil {
			return err
		}
		if verifierDIDs[v.Did] {
			return fmt.Errorf("genesis: duplicate verifier DID %s", v.Did)
		}
		verifierDIDs[v.Did] = true
		if err := ValidateAddr("verifier.signer_address", v.SignerAddress); err != nil {
			return err
		}
		if err := ValidateNonEmpty("verifier.name", v.Name, NameMaxLen); err != nil {
			return err
		}
		if uint32(len(v.Jurisdictions)) > gs.Params.MaxJurisdictionsPerVerifier {
			return fmt.Errorf("genesis: verifier %d too many jurisdictions", v.Id)
		}
		for _, j := range v.Jurisdictions {
			if err := ValidateJurisdiction("verifier.jurisdictions", j); err != nil {
				return err
			}
		}
		if uint32(len(v.AccreditedStandards)) > gs.Params.MaxStandardsPerVerifier {
			return fmt.Errorf("genesis: verifier %d too many standards", v.Id)
		}
	}

	// Reports must reference known schemas; if attested, must
	// reference a known verifier. Subjects, periods, and
	// hashes are structurally validated.
	reportIDs := map[uint64]bool{}
	reportsBySubject := map[string]uint32{}
	for _, r := range gs.Reports {
		if r.Id == 0 {
			return fmt.Errorf("genesis: report id must be > 0")
		}
		if reportIDs[r.Id] {
			return fmt.Errorf("genesis: duplicate report id %d", r.Id)
		}
		reportIDs[r.Id] = true
		if r.Id >= gs.NextReportId {
			return fmt.Errorf("genesis: report id %d >= next_report_id %d", r.Id, gs.NextReportId)
		}
		if !ReportStatusValid(r.Status) {
			return fmt.Errorf("genesis: report %d invalid status", r.Id)
		}
		if !schemaIDs[r.SchemaId] {
			return fmt.Errorf("genesis: report %d references unknown schema %d", r.Id, r.SchemaId)
		}
		if err := ValidateAddr("report.subject", r.Subject); err != nil {
			return err
		}
		if r.PeriodEnd <= r.PeriodStart {
			return fmt.Errorf("genesis: report %d period_end <= period_start", r.Id)
		}
		if err := ValidateURI("report.payload_uri", r.PayloadUri, gs.Params.UriMaxLen); err != nil {
			return err
		}
		if err := ValidateHash("report.payload_hash", r.PayloadHash); err != nil {
			return err
		}
		if r.VerifierId != 0 && !verifierIDs[r.VerifierId] {
			return fmt.Errorf("genesis: report %d references unknown verifier %d", r.Id, r.VerifierId)
		}
		// Attested reports must have a verifier set and a
		// verifier payload URI/hash recorded — otherwise the
		// ATTESTED status is a lie.
		if r.Status == ReportStatus_REPORT_STATUS_ATTESTED {
			if r.VerifierId == 0 {
				return fmt.Errorf("genesis: report %d ATTESTED but verifier_id=0", r.Id)
			}
			if err := ValidateURI("report.verifier_payload_uri", r.VerifierPayloadUri, gs.Params.UriMaxLen); err != nil {
				return err
			}
			if err := ValidateHash("report.verifier_payload_hash", r.VerifierPayloadHash); err != nil {
				return err
			}
		}
		if err := ValidateMemo(r.Memo, gs.Params.MemoMaxLen); err != nil {
			return err
		}
		reportsBySubject[r.Subject]++
		if reportsBySubject[r.Subject] > gs.Params.MaxReportsPerSubject {
			return fmt.Errorf("genesis: subject %s reports > max %d", r.Subject, gs.Params.MaxReportsPerSubject)
		}
	}

	if uint32(len(gs.Grants)) > gs.Params.MaxViewKeyGrants {
		return fmt.Errorf("genesis: %d grants > max %d", len(gs.Grants), gs.Params.MaxViewKeyGrants)
	}
	grantIDs := map[uint64]bool{}
	for _, g := range gs.Grants {
		if g.Id == 0 {
			return fmt.Errorf("genesis: grant id must be > 0")
		}
		if grantIDs[g.Id] {
			return fmt.Errorf("genesis: duplicate grant id %d", g.Id)
		}
		grantIDs[g.Id] = true
		if g.Id >= gs.NextGrantId {
			return fmt.Errorf("genesis: grant id %d >= next_grant_id %d", g.Id, gs.NextGrantId)
		}
		if err := ValidateAddr("grant.granter", g.Granter); err != nil {
			return err
		}
		if err := ValidateDID("grant.grantee_did", g.GranteeDid); err != nil {
			return err
		}
		if g.SchemaId != 0 && !schemaIDs[g.SchemaId] {
			return fmt.Errorf("genesis: grant %d references unknown schema %d", g.Id, g.SchemaId)
		}
		if g.ReportId != 0 && !reportIDs[g.ReportId] {
			return fmt.Errorf("genesis: grant %d references unknown report %d", g.Id, g.ReportId)
		}
		if g.ExpiresAt <= 0 {
			return fmt.Errorf("genesis: grant %d expires_at must be > 0", g.Id)
		}
	}
	return nil
}
