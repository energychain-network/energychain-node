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

	providers := map[string]bool{}
	for _, p := range gs.Providers {
		if _, err := sdk.AccAddressFromBech32(p.Address); err != nil {
			return fmt.Errorf("genesis: provider %q invalid address: %w", p.Address, err)
		}
		if providers[p.Address] {
			return fmt.Errorf("genesis: duplicate provider %q", p.Address)
		}
		providers[p.Address] = true
		if !ProviderRoleValid(p.Role) {
			return fmt.Errorf("genesis: provider %q invalid role", p.Address)
		}
		if !ProviderStatusValid(p.Status) {
			return fmt.Errorf("genesis: provider %q invalid status", p.Address)
		}
	}
	if uint32(len(gs.Providers)) > gs.Params.MaxProviders {
		return fmt.Errorf("genesis: providers exceed max")
	}

	devices := map[string]bool{}
	for _, d := range gs.Devices {
		if err := ValidateID(d.Id); err != nil {
			return fmt.Errorf("genesis: device %q: %w", d.Id, err)
		}
		if devices[d.Id] {
			return fmt.Errorf("genesis: duplicate device %q", d.Id)
		}
		devices[d.Id] = true
		if _, err := sdk.AccAddressFromBech32(d.Operator); err != nil {
			return fmt.Errorf("genesis: device %q invalid operator: %w", d.Id, err)
		}
		if !providers[d.Operator] {
			return fmt.Errorf("genesis: device %q references unknown operator %q", d.Id, d.Operator)
		}
		if !DeviceStatusValid(d.Status) {
			return fmt.Errorf("genesis: device %q invalid status", d.Id)
		}
	}
	if uint32(len(gs.Devices)) > gs.Params.MaxDevices {
		return fmt.Errorf("genesis: devices exceed max")
	}

	var maxReadingID uint64
	readingIDs := map[uint64]bool{}
	for _, r := range gs.Readings {
		if r.Id == 0 {
			return fmt.Errorf("genesis: reading id must be > 0")
		}
		if readingIDs[r.Id] {
			return fmt.Errorf("genesis: duplicate reading id %d", r.Id)
		}
		readingIDs[r.Id] = true
		if r.Id > maxReadingID {
			maxReadingID = r.Id
		}
		if !devices[r.DeviceId] {
			return fmt.Errorf("genesis: reading %d references unknown device %q", r.Id, r.DeviceId)
		}
	}
	if gs.ReadingIdSeq < maxReadingID {
		return fmt.Errorf("genesis: reading_id_seq %d < max reading id %d", gs.ReadingIdSeq, maxReadingID)
	}

	topics := map[string]bool{}
	for _, tpc := range gs.Topics {
		if err := ValidateTopicID(tpc.Id); err != nil {
			return fmt.Errorf("genesis: topic %q: %w", tpc.Id, err)
		}
		if topics[tpc.Id] {
			return fmt.Errorf("genesis: duplicate topic %q", tpc.Id)
		}
		topics[tpc.Id] = true
		if tpc.MinSources == 0 {
			return fmt.Errorf("genesis: topic %q min_sources must be > 0", tpc.Id)
		}
	}
	if uint32(len(gs.Topics)) > gs.Params.MaxTopics {
		return fmt.Errorf("genesis: topics exceed max")
	}

	subSeen := map[string]bool{}
	for _, s := range gs.Submissions {
		if !topics[s.TopicId] {
			return fmt.Errorf("genesis: submission references unknown topic %q", s.TopicId)
		}
		if !providers[s.Provider] {
			return fmt.Errorf("genesis: submission references unknown provider %q", s.Provider)
		}
		key := s.TopicId + "|" + s.Provider
		if subSeen[key] {
			return fmt.Errorf("genesis: duplicate submission %s", key)
		}
		subSeen[key] = true
	}

	return nil
}
