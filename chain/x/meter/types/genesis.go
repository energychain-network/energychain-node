package types

import (
	"encoding/hex"
	"fmt"
)

// DefaultParams returns the dev/testnet defaults. Production chains
// should review every value via governance: in particular,
// require_attested_device flips on in real deployments where x/device
// attestations gate the data path.
func DefaultParams() Params {
	return Params{
		MaxMetadataUriSize:    DefaultMaxMetadataURISize,
		MaxCommitmentSize:     DefaultMaxCommitmentSize,
		MaxBatchCount:         DefaultMaxBatchCount,
		MaxSignatureSize:      DefaultMaxSignatureSize,
		RequireAttestedDevice: false,
		StreamMaxValidity:     DefaultStreamMaxValidity,
		FutureSkewSeconds:     DefaultFutureSkewSeconds,
	}
}

func (p Params) Validate() error {
	if p.MaxMetadataUriSize == 0 || p.MaxMetadataUriSize > MaxMetadataURISizeUpper {
		return fmt.Errorf("max_metadata_uri_size %d outside (0, %d]",
			p.MaxMetadataUriSize, MaxMetadataURISizeUpper)
	}
	if p.MaxCommitmentSize == 0 || p.MaxCommitmentSize > MaxCommitmentSizeUpper {
		return fmt.Errorf("max_commitment_size %d outside (0, %d]",
			p.MaxCommitmentSize, MaxCommitmentSizeUpper)
	}
	if p.MaxBatchCount == 0 || p.MaxBatchCount > MaxBatchCountUpper {
		return fmt.Errorf("max_batch_count %d outside (0, %d]",
			p.MaxBatchCount, MaxBatchCountUpper)
	}
	if p.MaxSignatureSize == 0 || p.MaxSignatureSize > MaxSignatureSizeUpper {
		return fmt.Errorf("max_signature_size %d outside (0, %d]",
			p.MaxSignatureSize, MaxSignatureSizeUpper)
	}
	if p.StreamMaxValidity < 0 || p.StreamMaxValidity > StreamMaxValidityUpper {
		return fmt.Errorf("stream_max_validity %d outside [0, %d]",
			p.StreamMaxValidity, StreamMaxValidityUpper)
	}
	if p.FutureSkewSeconds < 0 || p.FutureSkewSeconds > FutureSkewSecondsUpper {
		return fmt.Errorf("future_skew_seconds %d outside [0, %d]",
			p.FutureSkewSeconds, FutureSkewSecondsUpper)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:          DefaultParams(),
		MeteringPoints:  []MeteringPoint{},
		Readings:        []Reading{},
		Batches:         []Batch{},
		Authorizations:  []StreamAuthorization{},
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("params: %w", err)
	}
	mpIDs := make(map[string]struct{}, len(gs.MeteringPoints))
	for i, mp := range gs.MeteringPoints {
		if err := validateMeteringPointShape(mp, gs.Params); err != nil {
			return fmt.Errorf("metering_point %d: %w", i, err)
		}
		if _, dup := mpIDs[mp.Id]; dup {
			return fmt.Errorf("metering_point %d: duplicate id %s", i, mp.Id)
		}
		mpIDs[mp.Id] = struct{}{}
	}

	readingSeen := make(map[string]struct{}, len(gs.Readings))
	for i, r := range gs.Readings {
		if err := validateReadingShape(r, gs.Params); err != nil {
			return fmt.Errorf("reading %d: %w", i, err)
		}
		if _, ok := mpIDs[r.MeteringPointId]; !ok {
			return fmt.Errorf("reading %d: references unknown metering_point %s",
				i, r.MeteringPointId)
		}
		key := fmt.Sprintf("%s|%d", r.MeteringPointId, r.StartTime)
		if _, dup := readingSeen[key]; dup {
			return fmt.Errorf("reading %d: duplicate (mp,start_time) %s", i, key)
		}
		readingSeen[key] = struct{}{}
	}

	batchIDs := make(map[string]struct{}, len(gs.Batches))
	for i, b := range gs.Batches {
		if err := validateBatchShape(b, gs.Params); err != nil {
			return fmt.Errorf("batch %d: %w", i, err)
		}
		if _, ok := mpIDs[b.MeteringPointId]; !ok {
			return fmt.Errorf("batch %d: references unknown metering_point %s",
				i, b.MeteringPointId)
		}
		if _, dup := batchIDs[b.Id]; dup {
			return fmt.Errorf("batch %d: duplicate id %s", i, b.Id)
		}
		batchIDs[b.Id] = struct{}{}
	}

	authIDs := make(map[string]struct{}, len(gs.Authorizations))
	for i, a := range gs.Authorizations {
		if a.Id == "" {
			return fmt.Errorf("authorization %d: empty id", i)
		}
		if _, dup := authIDs[a.Id]; dup {
			return fmt.Errorf("authorization %d: duplicate id %s", i, a.Id)
		}
		authIDs[a.Id] = struct{}{}
		if _, ok := mpIDs[a.MeteringPointId]; !ok {
			return fmt.Errorf("authorization %d: references unknown metering_point %s",
				i, a.MeteringPointId)
		}
		if a.DeviceDid == "" {
			return fmt.Errorf("authorization %d: empty device_did", i)
		}
		if a.AuthorizedBy == "" {
			return fmt.Errorf("authorization %d: empty authorized_by", i)
		}
		if a.IssuedAt < 0 || a.ExpiresAt < 0 {
			return fmt.Errorf("authorization %d: negative timestamps", i)
		}
		if a.ExpiresAt > 0 && a.IssuedAt > a.ExpiresAt {
			return fmt.Errorf("authorization %d: issued_at after expires_at", i)
		}
		if uint32(len(a.Metadata)) > StreamMetadataMaxLen {
			return fmt.Errorf("authorization %d: metadata too long", i)
		}
	}
	return nil
}

func validateMeteringPointShape(mp MeteringPoint, p Params) error {
	if err := ValidateMeteringPointID(mp.Id); err != nil {
		return err
	}
	if mp.OwnerAddress == "" {
		return fmt.Errorf("owner_address required")
	}
	if mp.DeviceDid == "" {
		return fmt.Errorf("device_did required")
	}
	if !MeasurementTypeValid(mp.MeasurementType) {
		return fmt.Errorf("invalid measurement_type %d", mp.MeasurementType)
	}
	if len(mp.GridZone) > GridZoneMaxLen {
		return fmt.Errorf("grid_zone too long")
	}
	if len(mp.GridOperator) > GridOperatorMaxLen {
		return fmt.Errorf("grid_operator too long")
	}
	if len(mp.Tariff) > TariffMaxLen {
		return fmt.Errorf("tariff too long")
	}
	if mp.Unit == "" || len(mp.Unit) > UnitMaxLen {
		return fmt.Errorf("unit length out of range")
	}
	if len(mp.Timezone) > TimezoneMaxLen {
		return fmt.Errorf("timezone too long")
	}
	if uint32(len(mp.MetadataUri)) > p.MaxMetadataUriSize {
		return fmt.Errorf("metadata_uri exceeds max %d", p.MaxMetadataUriSize)
	}
	return nil
}

func validateReadingShape(r Reading, p Params) error {
	if err := ValidateMeteringPointID(r.MeteringPointId); err != nil {
		return err
	}
	period, ok := ReadingPeriodSeconds(r.Period)
	if !ok {
		return fmt.Errorf("invalid period %d", r.Period)
	}
	if r.StartTime <= 0 || r.EndTime <= 0 {
		return fmt.Errorf("non-positive timestamps")
	}
	if r.EndTime-r.StartTime != period {
		return fmt.Errorf("end_time %d - start_time %d != period %ds",
			r.EndTime, r.StartTime, period)
	}
	if !CommitmentSchemeValid(r.CommitmentScheme) {
		return fmt.Errorf("invalid commitment_scheme %d", r.CommitmentScheme)
	}
	if uint32(len(r.CommitmentBytes)) == 0 || uint32(len(r.CommitmentBytes)) > p.MaxCommitmentSize {
		return fmt.Errorf("commitment_bytes length out of range")
	}
	if uint32(len(r.DeviceSignature)) > p.MaxSignatureSize {
		return fmt.Errorf("device_signature too long")
	}
	if !DataQualityValid(r.DataQuality) {
		return fmt.Errorf("invalid data_quality %d", r.DataQuality)
	}
	if r.SubmittedBy == "" {
		return fmt.Errorf("submitted_by required")
	}
	return nil
}

func validateBatchShape(b Batch, p Params) error {
	if b.Id == "" {
		return fmt.Errorf("batch id required")
	}
	if err := ValidateMeteringPointID(b.MeteringPointId); err != nil {
		return err
	}
	if len(b.MerkleRoot) != MerkleHashHexLen {
		return fmt.Errorf("merkle_root must be %d hex chars", MerkleHashHexLen)
	}
	if _, err := hex.DecodeString(b.MerkleRoot); err != nil {
		return fmt.Errorf("merkle_root must be hex")
	}
	if b.MerkleAlgorithm != MerkleAlgoSHA256 {
		return fmt.Errorf("merkle_algorithm %q unsupported (only %q today)",
			b.MerkleAlgorithm, MerkleAlgoSHA256)
	}
	if b.Count == 0 || b.Count > p.MaxBatchCount {
		return fmt.Errorf("count %d outside (0, %d]", b.Count, p.MaxBatchCount)
	}
	if _, ok := ReadingPeriodSeconds(b.Period); !ok {
		return fmt.Errorf("invalid period %d", b.Period)
	}
	if b.StartTime <= 0 || b.EndTime <= 0 || b.StartTime > b.EndTime {
		return fmt.Errorf("invalid time range")
	}
	if b.Uri == "" || len(b.Uri) > BatchURIMaxLen {
		return fmt.Errorf("uri length out of range")
	}
	if uint32(len(b.DeviceSignature)) > p.MaxSignatureSize {
		return fmt.Errorf("device_signature too long")
	}
	if b.SubmittedBy == "" {
		return fmt.Errorf("submitted_by required")
	}
	return nil
}
