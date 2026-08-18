package keeper

import (
	"context"
	"strconv"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/bridge/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

var _ types.MsgServer = msgServer{}

func u(v uint64) string { return strconv.FormatUint(v, 10) }

func (s msgServer) requireAuthority(addr string) error {
	if addr != s.k.authority {
		return types.ErrUnauthorized.Wrapf("expected authority %q", s.k.authority)
	}
	return nil
}

func (s msgServer) requireNotPaused(ctx context.Context) error {
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return err
	}
	if p.Paused {
		return types.ErrPaused
	}
	return nil
}

// ---- chain admin -----------------------------------------------------------

func (s msgServer) RegisterChain(ctx context.Context, m *types.MsgRegisterChain) (*types.MsgRegisterChainResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateAttestorSet(m.Attestors, m.Threshold, p.MaxAttestorsPerChain); err != nil {
		return nil, err
	}
	n, err := s.k.CountChains(ctx)
	if err != nil {
		return nil, err
	}
	if n >= p.MaxChains {
		return nil, types.ErrLimitExceeded.Wrap("max_chains")
	}
	id, err := nextSeq(ctx, s.k.ChainIDSeq)
	if err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	c := types.ExternalChain{
		Id: id, Name: m.Name, ChainRef: m.ChainRef, Attestors: m.Attestors,
		Threshold: m.Threshold, Status: types.ChainStatus_CHAIN_STATUS_ACTIVE, CreatedAt: sdkCtx.BlockTime().Unix(),
	}
	if err := s.k.Chains.Set(ctx, id, c); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeChain, types.AttrAction, "register", types.AttrChainID, u(id))
	return &types.MsgRegisterChainResponse{ChainId: id}, nil
}

func (s msgServer) UpdateChain(ctx context.Context, m *types.MsgUpdateChain) (*types.MsgUpdateChainResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateAttestorSet(m.Attestors, m.Threshold, p.MaxAttestorsPerChain); err != nil {
		return nil, err
	}
	c, ok, err := s.k.GetChain(ctx, m.ChainId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("chain %d", m.ChainId)
	}
	c.Attestors = m.Attestors
	c.Threshold = m.Threshold
	if err := s.k.Chains.Set(ctx, c.Id, c); err != nil {
		return nil, err
	}
	emitEvent(sdk.UnwrapSDKContext(ctx), types.EventTypeChain, types.AttrAction, "update", types.AttrChainID, u(c.Id))
	return &types.MsgUpdateChainResponse{}, nil
}

func (s msgServer) SetChainStatus(ctx context.Context, m *types.MsgSetChainStatus) (*types.MsgSetChainStatusResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if !types.ChainStatusValid(m.Status) {
		return nil, types.ErrInvalidField.Wrap("invalid status")
	}
	c, ok, err := s.k.GetChain(ctx, m.ChainId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("chain %d", m.ChainId)
	}
	c.Status = m.Status
	if err := s.k.Chains.Set(ctx, c.Id, c); err != nil {
		return nil, err
	}
	emitEvent(sdk.UnwrapSDKContext(ctx), types.EventTypeChain, types.AttrAction, "status", types.AttrChainID, u(c.Id), types.AttrStatus, m.Status.String())
	return &types.MsgSetChainStatusResponse{}, nil
}

// ---- asset admin -----------------------------------------------------------

func (s msgServer) RegisterAsset(ctx context.Context, m *types.MsgRegisterAsset) (*types.MsgRegisterAssetResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if err := types.ValidateDenom(m.Denom); err != nil {
		return nil, err
	}
	if has, err := s.k.AssetByDenom.Has(ctx, m.Denom); err != nil {
		return nil, err
	} else if has {
		return nil, types.ErrDuplicate.Wrapf("asset denom %s already registered", m.Denom)
	}
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	n, err := s.k.CountAssets(ctx)
	if err != nil {
		return nil, err
	}
	if n >= p.MaxAssets {
		return nil, types.ErrLimitExceeded.Wrap("max_assets")
	}
	id, err := nextSeq(ctx, s.k.AssetIDSeq)
	if err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	a := types.Asset{Id: id, Denom: m.Denom, Status: types.AssetStatus_ASSET_STATUS_ACTIVE, CreatedAt: sdkCtx.BlockTime().Unix()}
	if err := s.k.Assets.Set(ctx, id, a); err != nil {
		return nil, err
	}
	if err := s.k.AssetByDenom.Set(ctx, m.Denom, id); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeAsset, types.AttrAction, "register", types.AttrAssetID, u(id), types.AttrDenom, m.Denom)
	return &types.MsgRegisterAssetResponse{AssetId: id}, nil
}

func (s msgServer) SetAssetStatus(ctx context.Context, m *types.MsgSetAssetStatus) (*types.MsgSetAssetStatusResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if !types.AssetStatusValid(m.Status) {
		return nil, types.ErrInvalidField.Wrap("invalid status")
	}
	a, ok, err := s.k.GetAsset(ctx, m.AssetId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("asset %d", m.AssetId)
	}
	a.Status = m.Status
	if err := s.k.Assets.Set(ctx, a.Id, a); err != nil {
		return nil, err
	}
	emitEvent(sdk.UnwrapSDKContext(ctx), types.EventTypeAsset, types.AttrAction, "status", types.AttrAssetID, u(a.Id), types.AttrStatus, m.Status.String())
	return &types.MsgSetAssetStatusResponse{}, nil
}

// ---- lock (outbound) -------------------------------------------------------

func (s msgServer) Lock(ctx context.Context, m *types.MsgLock) (*types.MsgLockResponse, error) {
	if err := s.requireNotPaused(ctx); err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if p.MaxLockAmount != 0 && m.Amount > p.MaxLockAmount {
		return nil, types.ErrInvalidField.Wrapf("amount exceeds max_lock_amount %d", p.MaxLockAmount)
	}
	asset, ok, err := s.k.GetAsset(ctx, m.AssetId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("asset %d", m.AssetId)
	}
	if asset.Status != types.AssetStatus_ASSET_STATUS_ACTIVE {
		return nil, types.ErrAssetState.Wrapf("asset %d not active", asset.Id)
	}
	chain, ok, err := s.k.GetChain(ctx, m.DestChainId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("dest chain %d", m.DestChainId)
	}
	if chain.Status != types.ChainStatus_CHAIN_STATUS_ACTIVE {
		return nil, types.ErrChainState.Wrapf("dest chain %d not active", chain.Id)
	}
	if m.Amount == 0 {
		return nil, types.ErrInvalidField.Wrap("amount must be > 0")
	}
	// Burn the sender's native stablecoin; the off-chain relayer releases the
	// underlying collateral (USDC/USDT) on the destination chain. Sender
	// compliance and balance are enforced inside x/stableusd's BridgeBurn.
	if err := s.k.bridgeBurn(ctx, asset.Denom, m.Sender, m.Amount); err != nil {
		return nil, err
	}
	if err := s.k.recordBurn(ctx, asset.Denom, m.Amount); err != nil {
		return nil, err
	}
	nonce, err := nextSeq(ctx, s.k.OutboundSeq)
	if err != nil {
		return nil, err
	}
	o := types.Outbound{
		Nonce: nonce, Sender: m.Sender, AssetId: asset.Id, Amount: m.Amount,
		DestChainId: chain.Id, DestAddr: m.DestAddr, CreatedAt: sdkCtx.BlockTime().Unix(),
	}
	if err := s.k.Outbounds.Set(ctx, nonce, o); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeLock, types.AttrNonce, u(nonce), types.AttrSender, m.Sender,
		types.AttrAssetID, u(asset.Id), types.AttrAmount, u(m.Amount), types.AttrChainID, u(chain.Id))
	return &types.MsgLockResponse{Nonce: nonce}, nil
}

// ---- attest (inbound quorum) ----------------------------------------------

func (s msgServer) Attest(ctx context.Context, m *types.MsgAttest) (*types.MsgAttestResponse, error) {
	if err := s.requireNotPaused(ctx); err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	chain, ok, err := s.k.GetChain(ctx, m.SrcChainId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("src chain %d", m.SrcChainId)
	}
	if chain.Status != types.ChainStatus_CHAIN_STATUS_ACTIVE {
		return nil, types.ErrChainState.Wrapf("src chain %d not active", chain.Id)
	}
	if !types.Contains(chain.Attestors, m.Attestor) {
		return nil, types.ErrNotAttestor.Wrapf("%s not in chain %d attestor set", m.Attestor, chain.Id)
	}
	if _, ok, err := s.k.GetAsset(ctx, m.AssetId); err != nil {
		return nil, err
	} else if !ok {
		return nil, types.ErrNotFound.Wrapf("asset %d", m.AssetId)
	}

	srcKey := collections.Join(m.SrcChainId, m.SrcNonce)
	existingID, err := s.k.InboundBySrc.Get(ctx, srcKey)
	switch {
	case err == nil:
		// Existing inbound: accumulate the attestation.
		in, ok, err := s.k.GetInbound(ctx, existingID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, types.ErrInboundState.Wrapf("inbound %d index dangling", existingID)
		}
		if in.Status == types.InboundStatus_INBOUND_STATUS_RELEASED {
			return nil, types.ErrReplay.Wrapf("inbound %d already released", in.Id)
		}
		// The first attestation bound the canonical payload; everyone else must
		// match it exactly, otherwise a single attestor could forge a divergent
		// recipient/amount.
		if in.Recipient != m.Recipient || in.AssetId != m.AssetId || in.Amount != m.Amount {
			return nil, types.ErrPayloadMismatch.Wrapf("inbound %d payload differs from bound values", in.Id)
		}
		if types.Contains(in.Attestations, m.Attestor) {
			return nil, types.ErrAlreadyAttested.Wrapf("%s already attested inbound %d", m.Attestor, in.Id)
		}
		in.Attestations = append(in.Attestations, m.Attestor)
		// Quorum counts only attestations from current members (a rotated-out
		// attestor's stale signature must not contribute).
		if in.Status == types.InboundStatus_INBOUND_STATUS_PENDING &&
			CountValidAttestations(in.Attestations, chain.Attestors) >= chain.Threshold {
			in.Status = types.InboundStatus_INBOUND_STATUS_ATTESTED
		}
		if err := s.k.Inbounds.Set(ctx, in.Id, in); err != nil {
			return nil, err
		}
		emitEvent(sdkCtx, types.EventTypeAttest, types.AttrInboundID, u(in.Id), types.AttrAttestor, m.Attestor, types.AttrStatus, in.Status.String())
		return &types.MsgAttestResponse{InboundId: in.Id, Status: in.Status}, nil

	case errIsNotFound(err):
		// First attestation: create the inbound and bind the payload.
		id, err := nextSeq(ctx, s.k.InboundIDSeq)
		if err != nil {
			return nil, err
		}
		in := types.Inbound{
			Id: id, SrcChainId: m.SrcChainId, SrcNonce: m.SrcNonce, Recipient: m.Recipient,
			AssetId: m.AssetId, Amount: m.Amount, Attestations: []string{m.Attestor},
			Status: types.InboundStatus_INBOUND_STATUS_PENDING, CreatedAt: sdkCtx.BlockTime().Unix(),
		}
		if CountValidAttestations(in.Attestations, chain.Attestors) >= chain.Threshold {
			in.Status = types.InboundStatus_INBOUND_STATUS_ATTESTED
		}
		if err := s.k.Inbounds.Set(ctx, id, in); err != nil {
			return nil, err
		}
		if err := s.k.InboundBySrc.Set(ctx, srcKey, id); err != nil {
			return nil, err
		}
		emitEvent(sdkCtx, types.EventTypeAttest, types.AttrInboundID, u(id), types.AttrAttestor, m.Attestor, types.AttrStatus, in.Status.String())
		return &types.MsgAttestResponse{InboundId: id, Status: in.Status}, nil

	default:
		return nil, err
	}
}

// ---- release (inbound payout) ----------------------------------------------

func (s msgServer) Release(ctx context.Context, m *types.MsgRelease) (*types.MsgReleaseResponse, error) {
	p, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if p.Paused {
		return nil, types.ErrPaused
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	in, ok, err := s.k.GetInbound(ctx, m.InboundId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("inbound %d", m.InboundId)
	}
	if in.Status == types.InboundStatus_INBOUND_STATUS_RELEASED {
		return nil, types.ErrReplay.Wrapf("inbound %d already released", in.Id)
	}
	// Re-evaluate quorum at release time against the CURRENT attestor set and
	// threshold. This is the authoritative gate: it picks up a governance
	// threshold drop (unsticking a transfer that already has enough current
	// signatures) and refuses release if a rotation removed attestors below
	// quorum — independent of the cached PENDING/ATTESTED status.
	chain, ok, err := s.k.GetChain(ctx, in.SrcChainId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("src chain %d", in.SrcChainId)
	}
	if chain.Status != types.ChainStatus_CHAIN_STATUS_ACTIVE {
		return nil, types.ErrChainState.Wrapf("src chain %d not active", chain.Id)
	}
	if CountValidAttestations(in.Attestations, chain.Attestors) < chain.Threshold {
		return nil, types.ErrInboundState.Wrapf("inbound %d below current quorum", in.Id)
	}
	asset, ok, err := s.k.GetAsset(ctx, in.AssetId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, types.ErrNotFound.Wrapf("asset %d", in.AssetId)
	}
	if asset.Status != types.AssetStatus_ASSET_STATUS_ACTIVE {
		return nil, types.ErrAssetState.Wrapf("asset %d not active", asset.Id)
	}
	// Mint rate limits bound the loss from a compromised attestor quorum in
	// TIME; the reserve gate inside x/stableusd bounds it in TOTAL. Per-tx cap:
	if p.MaxMintPerTx != 0 && in.Amount > p.MaxMintPerTx {
		return nil, types.ErrLimitExceeded.Wrapf("amount %d exceeds max_mint_per_tx %d", in.Amount, p.MaxMintPerTx)
	}
	// Rolling per-source-chain window cap: how much this chain has already
	// minted within the window (excluding this still-unreleased inbound) plus
	// the current amount must stay under the cap.
	now := sdkCtx.BlockTime().Unix()
	if p.MintWindowSeconds != 0 && p.MaxMintPerWindow != 0 {
		since := now - int64(p.MintWindowSeconds)
		prior, werr := s.k.mintedInWindow(ctx, chain.Id, since)
		if werr != nil {
			return nil, werr
		}
		projected, aerr := types.SafeAdd(prior, in.Amount)
		if aerr != nil {
			return nil, aerr
		}
		if projected > p.MaxMintPerWindow {
			return nil, types.ErrLimitExceeded.Wrapf("chain %d mint window exceeded: %d + %d > %d",
				chain.Id, prior, in.Amount, p.MaxMintPerWindow)
		}
	}
	// Mint the native stablecoin 1:1 to the recipient. Reserve coverage and
	// recipient compliance are enforced inside x/stableusd's BridgeMint, so a
	// release can never issue beyond the attested off-chain collateral.
	if err := s.k.bridgeMint(ctx, asset.Denom, in.Recipient, in.Amount); err != nil {
		return nil, err
	}
	in.Status = types.InboundStatus_INBOUND_STATUS_RELEASED
	in.ReleasedAt = now
	if err := s.k.Inbounds.Set(ctx, in.Id, in); err != nil {
		return nil, err
	}
	if err := s.k.recordRelease(ctx, in, asset.Denom); err != nil {
		return nil, err
	}
	emitEvent(sdkCtx, types.EventTypeRelease, types.AttrInboundID, u(in.Id), types.AttrRecipient, in.Recipient,
		types.AttrAssetID, u(asset.Id), types.AttrAmount, u(in.Amount))
	return &types.MsgReleaseResponse{}, nil
}

func (s msgServer) UpdateParams(ctx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
