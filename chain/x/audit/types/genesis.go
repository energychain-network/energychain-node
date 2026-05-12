package types

import (
	"fmt"
)

// DefaultParams returns the default audit module parameters.
//
// Permissionless mirrors the energy module: when true (default for dev),
// any address can record audit logs; when false, only addresses listed in
// AllowedAuditors are accepted.
func DefaultParams() Params {
	return Params{
		LargeTransferThreshold:      1_000_000,
		RetentionBlocks:             0,
		AllowedAuditors:             []string{},
		MaxDataSize:                 DefaultAuditMaxData,
		Permissionless:              true,
		MaxAuditsPerBlockPerCreator: DefaultAuditMaxPerBlock,

		// M1 evolution defaults.
		ArchiveMinAgeBlocks: DefaultArchiveMinAgeBlocks,
		ViewKeyMaxValidity:  DefaultViewKeyMaxValidity,
		ArchiveMaxBatch:     DefaultArchiveMaxBatch,
		ArchiveAuthorities:  []string{},
	}
}

func (p Params) Validate() error {
	if p.RetentionBlocks < 0 {
		return fmt.Errorf("retention_blocks must be non-negative, got %d", p.RetentionBlocks)
	}
	if p.RetentionBlocks > AuditRetentionUpperBound {
		return fmt.Errorf("retention_blocks %d exceeds hard upper bound %d",
			p.RetentionBlocks, AuditRetentionUpperBound)
	}
	if p.MaxDataSize < AuditMaxDataLowerBound {
		return fmt.Errorf("max_data_size %d is below hard lower bound %d",
			p.MaxDataSize, AuditMaxDataLowerBound)
	}
	if p.MaxDataSize > AuditMaxDataUpperBound {
		return fmt.Errorf("max_data_size %d exceeds hard upper bound %d (1 MiB)",
			p.MaxDataSize, AuditMaxDataUpperBound)
	}
	if p.MaxAuditsPerBlockPerCreator > AuditMaxAuditsPerBlockUpper {
		return fmt.Errorf("max_audits_per_block_per_creator %d exceeds hard upper bound %d",
			p.MaxAuditsPerBlockPerCreator, AuditMaxAuditsPerBlockUpper)
	}
	if p.ArchiveMinAgeBlocks < 0 {
		return fmt.Errorf("archive_min_age_blocks must be non-negative, got %d", p.ArchiveMinAgeBlocks)
	}
	if p.ArchiveMinAgeBlocks > ArchiveMinAgeUpperBound {
		return fmt.Errorf("archive_min_age_blocks %d exceeds hard upper bound %d",
			p.ArchiveMinAgeBlocks, ArchiveMinAgeUpperBound)
	}
	if p.ViewKeyMaxValidity < 0 {
		return fmt.Errorf("view_key_max_validity must be non-negative, got %d", p.ViewKeyMaxValidity)
	}
	if p.ViewKeyMaxValidity > ViewKeyMaxValidityUpper {
		return fmt.Errorf("view_key_max_validity %d exceeds hard upper bound %d",
			p.ViewKeyMaxValidity, ViewKeyMaxValidityUpper)
	}
	if p.ArchiveMaxBatch == 0 || p.ArchiveMaxBatch > ArchiveMaxBatchUpper {
		return fmt.Errorf("archive_max_batch %d outside (0, %d]",
			p.ArchiveMaxBatch, ArchiveMaxBatchUpper)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:          DefaultParams(),
		Logs:            []AuditLog{},
		Counter:         0,
		Schemas:         []SchemaDescriptor{},
		ArchiveSegments: []ArchiveSegment{},
		ArchiveCounter:  0,
		ViewKeyGrants:   []ViewKeyGrant{},
		GrantCounter:    0,
	}
}

// Validate enforces structural soundness of the genesis blob: unique
// primary keys, payload bounds, and the basic invariants the keeper
// relies on (e.g. archive segments do not overlap each other).
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	if err := validateLogs(gs.Logs, gs.Params); err != nil {
		return err
	}
	if err := validateSchemas(gs.Schemas); err != nil {
		return err
	}
	if err := validateArchiveSegments(gs.ArchiveSegments); err != nil {
		return err
	}
	if err := validateViewKeyGrants(gs.ViewKeyGrants); err != nil {
		return err
	}
	return nil
}

func validateLogs(logs []AuditLog, params Params) error {
	seen := make(map[uint64]bool, len(logs))
	for i, log := range logs {
		if seen[log.ID] {
			return fmt.Errorf("duplicate audit log ID %d at index %d", log.ID, i)
		}
		seen[log.ID] = true

		if log.Actor == "" {
			return fmt.Errorf("audit log %d has empty actor", log.ID)
		}
		if log.Action == "" {
			return fmt.Errorf("audit log %d has empty action", log.ID)
		}
		if log.EventType == "" {
			return fmt.Errorf("audit log %d has empty event_type", log.ID)
		}
		if uint32(len(log.Data)) > params.MaxDataSize {
			return fmt.Errorf("audit log %d data size %d exceeds max %d",
				log.ID, len(log.Data), params.MaxDataSize)
		}
		if !SeverityValid(log.Severity) {
			return fmt.Errorf("audit log %d has unknown severity %d", log.ID, log.Severity)
		}
		if log.PayloadEncrypted && log.PayloadDigest == "" {
			return fmt.Errorf("audit log %d marked encrypted but missing payload_digest", log.ID)
		}
		if log.PayloadDigest != "" && len(log.PayloadDigest) != SchemaHashHexLen {
			return fmt.Errorf("audit log %d payload_digest must be %d hex chars, got %d",
				log.ID, SchemaHashHexLen, len(log.PayloadDigest))
		}
	}
	return nil
}

func validateSchemas(descriptors []SchemaDescriptor) error {
	seen := make(map[string]bool, len(descriptors))
	for i, d := range descriptors {
		if d.EventType == "" {
			return fmt.Errorf("schema %d: event_type empty", i)
		}
		if seen[d.EventType] {
			return fmt.Errorf("schema %d: duplicate event_type %q", i, d.EventType)
		}
		seen[d.EventType] = true
		if len(d.Uri) == 0 || len(d.Uri) > SchemaURIMaxLen {
			return fmt.Errorf("schema %s: uri length %d outside (0, %d]",
				d.EventType, len(d.Uri), SchemaURIMaxLen)
		}
		if len(d.Hash) != SchemaHashHexLen {
			return fmt.Errorf("schema %s: hash must be %d hex chars, got %d",
				d.EventType, SchemaHashHexLen, len(d.Hash))
		}
	}
	return nil
}

// validateArchiveSegments enforces unique IDs and non-overlapping log id
// ranges. We intentionally allow gaps (e.g. live logs sit between two
// archived spans) because the archive job may skip not-yet-aged logs.
func validateArchiveSegments(segments []ArchiveSegment) error {
	seen := make(map[uint64]bool, len(segments))
	type span struct{ from, to uint64 }
	spans := make([]span, 0, len(segments))
	for i, s := range segments {
		if seen[s.ID] {
			return fmt.Errorf("archive segment %d: duplicate id", s.ID)
		}
		seen[s.ID] = true
		if s.FromLogId == 0 || s.ToLogId == 0 || s.FromLogId > s.ToLogId {
			return fmt.Errorf("archive segment %d: invalid range [%d, %d]",
				s.ID, s.FromLogId, s.ToLogId)
		}
		if s.MerkleRoot == "" {
			return fmt.Errorf("archive segment %d: empty merkle_root", s.ID)
		}
		if s.Uri == "" {
			return fmt.Errorf("archive segment %d: empty uri", s.ID)
		}
		for j, prior := range spans {
			if s.FromLogId <= prior.to && s.ToLogId >= prior.from {
				return fmt.Errorf("archive segment %d overlaps segment at index %d", i, j)
			}
		}
		spans = append(spans, span{from: s.FromLogId, to: s.ToLogId})
	}
	return nil
}

func validateViewKeyGrants(grants []ViewKeyGrant) error {
	seen := make(map[string]bool, len(grants))
	for i, g := range grants {
		if g.Id == "" {
			return fmt.Errorf("view key grant %d: id empty", i)
		}
		if seen[g.Id] {
			return fmt.Errorf("view key grant %d: duplicate id %q", i, g.Id)
		}
		seen[g.Id] = true
		if g.Grantor == "" {
			return fmt.Errorf("view key grant %s: grantor empty", g.Id)
		}
		if g.Grantee == "" {
			return fmt.Errorf("view key grant %s: grantee empty", g.Id)
		}
		if len(g.EncryptedKey) == 0 || len(g.EncryptedKey) > GrantEncryptedKeyMaxLen {
			return fmt.Errorf("view key grant %s: encrypted_key length %d outside (0, %d]",
				g.Id, len(g.EncryptedKey), GrantEncryptedKeyMaxLen)
		}
		if g.ExpiresAt < 0 {
			return fmt.Errorf("view key grant %s: expires_at must be non-negative", g.Id)
		}
	}
	return nil
}
