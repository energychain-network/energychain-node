package types

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

// DefaultParams returns the dev/testnet defaults. Production chains MUST
// override BondMinAmount via governance (the default is 0 to keep tests
// hassle-free).
func DefaultParams() Params {
	return Params{
		MinSubmissions:         DefaultMinSubmissions,
		DataMaxAge:             DefaultDataMaxAge,
		MaxMetadataSize:        DefaultMaxMetadataSize,
		BondDenom:              DefaultBondDenom,
		BondMinAmount:          "0",
		BondReleaseCooldown:    DefaultBondReleaseCool,
		MaxTopicsPerBlock:      DefaultMaxTopicsPerBlock,
		MaxSubmissionsPerTopic: DefaultMaxSubmissionsPerTopic,
	}
}

func (p Params) Validate() error {
	if p.MaxMetadataSize == 0 || p.MaxMetadataSize > MaxMetadataSizeUpper {
		return fmt.Errorf("max_metadata_size %d outside (0, %d]",
			p.MaxMetadataSize, MaxMetadataSizeUpper)
	}
	if p.BondReleaseCooldown < 0 || p.BondReleaseCooldown > BondReleaseCooldownUpper {
		return fmt.Errorf("bond_release_cooldown %d outside [0, %d]",
			p.BondReleaseCooldown, BondReleaseCooldownUpper)
	}
	if p.MaxTopicsPerBlock == 0 || p.MaxTopicsPerBlock > MaxTopicsPerBlockUpper {
		return fmt.Errorf("max_topics_per_block %d outside (0, %d]",
			p.MaxTopicsPerBlock, MaxTopicsPerBlockUpper)
	}
	if p.MaxSubmissionsPerTopic == 0 || p.MaxSubmissionsPerTopic > MaxSubmissionsPerTopicUpper {
		return fmt.Errorf("max_submissions_per_topic %d outside (0, %d]",
			p.MaxSubmissionsPerTopic, MaxSubmissionsPerTopicUpper)
	}
	if p.BondDenom == "" {
		return fmt.Errorf("bond_denom required")
	}
	if _, err := ParseAmountString(p.BondMinAmount); err != nil {
		return fmt.Errorf("bond_min_amount: %w", err)
	}
	return nil
}

// BondMinInt parses BondMinAmount; called from msg_server / keeper at
// runtime. Validate has already enforced parseability so this never
// fails on stored params; a 0 fallback is returned defensively.
func (p Params) BondMinInt() sdkmath.Int {
	v, err := ParseAmountString(p.BondMinAmount)
	if err != nil {
		return sdkmath.ZeroInt()
	}
	return v
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:              DefaultParams(),
		Topics:              []Topic{},
		Providers:           []Provider{},
		Submissions:         []Submission{},
		Aggregated:          []AggregatedValue{},
		ReserveAttestations: []ReserveAttestation{},
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("params: %w", err)
	}

	topicIDs := make(map[string]bool, len(gs.Topics))
	for i, t := range gs.Topics {
		if err := ValidateTopicID(t.Id); err != nil {
			return fmt.Errorf("topic %d: %w", i, err)
		}
		if topicIDs[t.Id] {
			return fmt.Errorf("topic %d: duplicate id %q", i, t.Id)
		}
		topicIDs[t.Id] = true
		if !TopicKindValid(t.Kind) {
			return fmt.Errorf("topic %s: unknown kind %d", t.Id, t.Kind)
		}
		if !AggregationFnValid(t.Aggregation) {
			return fmt.Errorf("topic %s: unknown aggregation %d", t.Id, t.Aggregation)
		}
		if t.OutlierBandBps > OutlierBandUpperBps {
			return fmt.Errorf("topic %s: outlier_band_bps %d exceeds %d", t.Id, t.OutlierBandBps, OutlierBandUpperBps)
		}
		if t.ValueDecimals > ValueDecimalsUpper {
			return fmt.Errorf("topic %s: value_decimals %d exceeds %d", t.Id, t.ValueDecimals, ValueDecimalsUpper)
		}
		if len(t.Quote) > QuoteMaxLen {
			return fmt.Errorf("topic %s: quote too long", t.Id)
		}
	}

	providerSeen := make(map[string]bool, len(gs.Providers))
	for i, p := range gs.Providers {
		if p.Address == "" {
			return fmt.Errorf("provider %d: empty address", i)
		}
		if providerSeen[p.Address] {
			return fmt.Errorf("provider %d: duplicate address %s", i, p.Address)
		}
		providerSeen[p.Address] = true
		if !ProviderStatusValid(p.Status) {
			return fmt.Errorf("provider %s: invalid status", p.Address)
		}
		if p.Bond.Denom != gs.Params.BondDenom {
			return fmt.Errorf("provider %s: bond denom %s != params %s",
				p.Address, p.Bond.Denom, gs.Params.BondDenom)
		}
		if p.Bond.IsNegative() {
			return fmt.Errorf("provider %s: negative bond", p.Address)
		}
	}

	subSeen := make(map[string]bool, len(gs.Submissions))
	for i, s := range gs.Submissions {
		if s.TopicId == "" || s.Provider == "" {
			return fmt.Errorf("submission %d: empty topic_id or provider", i)
		}
		if !topicIDs[s.TopicId] {
			return fmt.Errorf("submission %d: references unknown topic %s", i, s.TopicId)
		}
		if !providerSeen[s.Provider] {
			return fmt.Errorf("submission %d: references unknown provider %s", i, s.Provider)
		}
		key := s.TopicId + "|" + s.Provider
		if subSeen[key] {
			return fmt.Errorf("submission %d: duplicate (topic, provider)", i)
		}
		subSeen[key] = true
		if uint32(len(s.Metadata)) > gs.Params.MaxMetadataSize {
			return fmt.Errorf("submission %d: metadata exceeds max", i)
		}
	}

	aggSeen := make(map[string]bool, len(gs.Aggregated))
	for i, a := range gs.Aggregated {
		if aggSeen[a.TopicId] {
			return fmt.Errorf("aggregated %d: duplicate topic %s", i, a.TopicId)
		}
		aggSeen[a.TopicId] = true
		if !topicIDs[a.TopicId] {
			return fmt.Errorf("aggregated %d: references unknown topic %s", i, a.TopicId)
		}
	}

	resSeen := make(map[string]bool, len(gs.ReserveAttestations))
	for i, r := range gs.ReserveAttestations {
		if r.Id == "" {
			return fmt.Errorf("reserve attestation %d: empty id", i)
		}
		if resSeen[r.Id] {
			return fmt.Errorf("reserve attestation %d: duplicate id %s", i, r.Id)
		}
		resSeen[r.Id] = true
		if r.Threshold == 0 || int(r.Threshold) > len(r.Signers) {
			return fmt.Errorf("reserve attestation %s: threshold %d outside (0, %d]",
				r.Id, r.Threshold, len(r.Signers))
		}
		if _, err := ParseAmountString(r.Amount); err != nil {
			return fmt.Errorf("reserve attestation %s: %w", r.Id, err)
		}
	}
	return nil
}
