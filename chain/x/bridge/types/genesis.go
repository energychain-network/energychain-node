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

	// chains
	chainThreshold := map[uint64]uint32{}
	chainAttestors := map[uint64][]string{}
	var maxChain uint64
	for _, c := range gs.Chains {
		if c.Id == 0 {
			return fmt.Errorf("genesis: chain id must be > 0")
		}
		if _, dup := chainThreshold[c.Id]; dup {
			return fmt.Errorf("genesis: duplicate chain id %d", c.Id)
		}
		if err := ValidateMaxLen("name", c.Name, NameMaxLen); err != nil {
			return fmt.Errorf("genesis: chain %d: %w", c.Id, err)
		}
		if err := ValidateMaxLen("chain_ref", c.ChainRef, ChainRefMaxLen); err != nil {
			return fmt.Errorf("genesis: chain %d: %w", c.Id, err)
		}
		if err := ValidateAttestorSet(c.Attestors, c.Threshold, gs.Params.MaxAttestorsPerChain); err != nil {
			return fmt.Errorf("genesis: chain %d: %w", c.Id, err)
		}
		if !ChainStatusValid(c.Status) {
			return fmt.Errorf("genesis: chain %d invalid status", c.Id)
		}
		chainThreshold[c.Id] = c.Threshold
		chainAttestors[c.Id] = c.Attestors
		if c.Id > maxChain {
			maxChain = c.Id
		}
	}
	if uint32(len(gs.Chains)) > gs.Params.MaxChains {
		return fmt.Errorf("genesis: chains exceed max")
	}

	// assets
	assetDenom := map[uint64]string{}
	denomSeen := map[string]bool{}
	var maxAsset uint64
	for _, a := range gs.Assets {
		if a.Id == 0 {
			return fmt.Errorf("genesis: asset id must be > 0")
		}
		if _, dup := assetDenom[a.Id]; dup {
			return fmt.Errorf("genesis: duplicate asset id %d", a.Id)
		}
		if err := ValidateDenom(a.Denom); err != nil {
			return fmt.Errorf("genesis: asset %d: %w", a.Id, err)
		}
		if denomSeen[a.Denom] {
			return fmt.Errorf("genesis: duplicate asset denom %s", a.Denom)
		}
		if !AssetStatusValid(a.Status) {
			return fmt.Errorf("genesis: asset %d invalid status", a.Id)
		}
		assetDenom[a.Id] = a.Denom
		denomSeen[a.Denom] = true
		if a.Id > maxAsset {
			maxAsset = a.Id
		}
	}
	if uint32(len(gs.Assets)) > gs.Params.MaxAssets {
		return fmt.Errorf("genesis: assets exceed max")
	}

	// outbounds
	nonceSeen := map[uint64]bool{}
	var maxNonce uint64
	for _, o := range gs.Outbounds {
		if o.Nonce == 0 {
			return fmt.Errorf("genesis: outbound nonce must be > 0")
		}
		if nonceSeen[o.Nonce] {
			return fmt.Errorf("genesis: duplicate outbound nonce %d", o.Nonce)
		}
		nonceSeen[o.Nonce] = true
		if _, err := sdk.AccAddressFromBech32(o.Sender); err != nil {
			return fmt.Errorf("genesis: outbound %d sender: %w", o.Nonce, err)
		}
		if _, ok := assetDenom[o.AssetId]; !ok {
			return fmt.Errorf("genesis: outbound %d unknown asset %d", o.Nonce, o.AssetId)
		}
		if o.Amount == 0 {
			return fmt.Errorf("genesis: outbound %d amount must be > 0", o.Nonce)
		}
		if _, ok := chainThreshold[o.DestChainId]; !ok {
			return fmt.Errorf("genesis: outbound %d unknown dest chain %d", o.Nonce, o.DestChainId)
		}
		if o.Nonce > maxNonce {
			maxNonce = o.Nonce
		}
	}

	// inbounds
	inboundIDs := map[uint64]bool{}
	srcSeen := map[[2]uint64]bool{}
	var maxInbound uint64
	for _, in := range gs.Inbounds {
		if in.Id == 0 {
			return fmt.Errorf("genesis: inbound id must be > 0")
		}
		if inboundIDs[in.Id] {
			return fmt.Errorf("genesis: duplicate inbound id %d", in.Id)
		}
		inboundIDs[in.Id] = true
		key := [2]uint64{in.SrcChainId, in.SrcNonce}
		if srcSeen[key] {
			return fmt.Errorf("genesis: duplicate inbound source (%d,%d)", in.SrcChainId, in.SrcNonce)
		}
		srcSeen[key] = true
		threshold, ok := chainThreshold[in.SrcChainId]
		if !ok {
			return fmt.Errorf("genesis: inbound %d unknown src chain %d", in.Id, in.SrcChainId)
		}
		if _, err := sdk.AccAddressFromBech32(in.Recipient); err != nil {
			return fmt.Errorf("genesis: inbound %d recipient: %w", in.Id, err)
		}
		if _, ok := assetDenom[in.AssetId]; !ok {
			return fmt.Errorf("genesis: inbound %d unknown asset %d", in.Id, in.AssetId)
		}
		if in.Amount == 0 {
			return fmt.Errorf("genesis: inbound %d amount must be > 0", in.Id)
		}
		if !InboundStatusValid(in.Status) {
			return fmt.Errorf("genesis: inbound %d invalid status", in.Id)
		}
		// distinct attestations, each a member of the src chain's attestor set
		seen := map[string]bool{}
		for _, at := range in.Attestations {
			if _, err := sdk.AccAddressFromBech32(at); err != nil {
				return fmt.Errorf("genesis: inbound %d attestation: %w", in.Id, err)
			}
			if seen[at] {
				return fmt.Errorf("genesis: inbound %d duplicate attestation %s", in.Id, at)
			}
			if !Contains(chainAttestors[in.SrcChainId], at) {
				return fmt.Errorf("genesis: inbound %d attestation %s not in chain %d attestor set", in.Id, at, in.SrcChainId)
			}
			seen[at] = true
		}
		count := uint32(len(in.Attestations))
		switch in.Status {
		case InboundStatus_INBOUND_STATUS_PENDING:
			if count >= threshold {
				return fmt.Errorf("genesis: pending inbound %d already has quorum", in.Id)
			}
		case InboundStatus_INBOUND_STATUS_ATTESTED, InboundStatus_INBOUND_STATUS_RELEASED:
			if count < threshold {
				return fmt.Errorf("genesis: inbound %d below quorum for status", in.Id)
			}
		}
		if in.Id > maxInbound {
			maxInbound = in.Id
		}
	}

	if gs.ChainIdSeq < maxChain {
		return fmt.Errorf("genesis: chain_id_seq %d < max %d", gs.ChainIdSeq, maxChain)
	}
	if gs.AssetIdSeq < maxAsset {
		return fmt.Errorf("genesis: asset_id_seq %d < max %d", gs.AssetIdSeq, maxAsset)
	}
	if gs.OutboundNonceSeq < maxNonce {
		return fmt.Errorf("genesis: outbound_nonce_seq %d < max %d", gs.OutboundNonceSeq, maxNonce)
	}
	if gs.InboundIdSeq < maxInbound {
		return fmt.Errorf("genesis: inbound_id_seq %d < max %d", gs.InboundIdSeq, maxInbound)
	}
	return nil
}
