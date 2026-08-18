package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/bridge/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	compliance types.ComplianceKeeper
	settlement types.SettlementKeeper

	Schema collections.Schema

	Params       collections.Item[types.Params]
	Chains       collections.Map[uint64, types.ExternalChain]
	ChainIDSeq   collections.Sequence
	Assets       collections.Map[uint64, types.Asset]
	AssetByDenom collections.Map[string, uint64]
	AssetIDSeq   collections.Sequence
	Outbounds    collections.Map[uint64, types.Outbound]
	OutboundSeq  collections.Sequence
	Inbounds     collections.Map[uint64, types.Inbound]
	InboundBySrc collections.Map[collections.Pair[uint64, uint64], uint64]
	InboundIDSeq collections.Sequence

	// ReleasedByChain: (src_chain_id, released_at, inbound_id) -> amount.
	// Time-ordered index over RELEASED inbounds so the rolling mint-window
	// limit reads only the window, not the whole history.
	ReleasedByChain collections.Map[collections.Triple[uint64, uint64, uint64], uint64]
	// MintedTotal / BurnedTotal: denom -> cumulative released-mint / burn
	// amounts, kept in lockstep with Release / Lock so net outstanding is
	// an O(1) read.
	MintedTotal collections.Map[string, uint64]
	BurnedTotal collections.Map[string, uint64]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	compliance types.ComplianceKeeper,
	settlement types.SettlementKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("bridge: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		compliance:   compliance,
		settlement:   settlement,

		Params:       collections.NewItem(sb, types.ParamsPrefix, "params", codec.CollValue[types.Params](cdc)),
		Chains:       collections.NewMap(sb, types.ChainPrefix, "chains", collections.Uint64Key, codec.CollValue[types.ExternalChain](cdc)),
		ChainIDSeq:   collections.NewSequence(sb, types.ChainIDSeqPrefix, "chain_id_seq"),
		Assets:       collections.NewMap(sb, types.AssetPrefix, "assets", collections.Uint64Key, codec.CollValue[types.Asset](cdc)),
		AssetByDenom: collections.NewMap(sb, types.AssetByDenomPrefix, "asset_by_denom", collections.StringKey, collections.Uint64Value),
		AssetIDSeq:   collections.NewSequence(sb, types.AssetIDSeqPrefix, "asset_id_seq"),
		Outbounds:    collections.NewMap(sb, types.OutboundPrefix, "outbounds", collections.Uint64Key, codec.CollValue[types.Outbound](cdc)),
		OutboundSeq:  collections.NewSequence(sb, types.OutboundSeqPrefix, "outbound_seq"),
		Inbounds:     collections.NewMap(sb, types.InboundPrefix, "inbounds", collections.Uint64Key, codec.CollValue[types.Inbound](cdc)),
		InboundBySrc: collections.NewMap(sb, types.InboundBySrcPrefix, "inbound_by_src",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key), collections.Uint64Value),
		InboundIDSeq: collections.NewSequence(sb, types.InboundIDSeqPrefix, "inbound_id_seq"),
		ReleasedByChain: collections.NewMap(sb, types.ReleasedByChainPrefix, "released_by_chain",
			collections.TripleKeyCodec(collections.Uint64Key, collections.Uint64Key, collections.Uint64Key),
			collections.Uint64Value),
		MintedTotal: collections.NewMap(sb, types.MintedTotalPrefix, "minted_total", collections.StringKey, collections.Uint64Value),
		BurnedTotal: collections.NewMap(sb, types.BurnedTotalPrefix, "burned_total", collections.StringKey, collections.Uint64Value),
	}
	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ---- params ---------------------------------------------------------------

func (k Keeper) SetParams(ctx context.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return k.Params.Set(ctx, p)
}

func (k Keeper) GetParams(ctx context.Context) (types.Params, error) {
	p, err := k.Params.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.DefaultParams(), nil
		}
		return types.Params{}, err
	}
	return p, nil
}

// ---- chains ----------------------------------------------------------------

func (k Keeper) GetChain(ctx context.Context, id uint64) (types.ExternalChain, bool, error) {
	c, err := k.Chains.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.ExternalChain{}, false, nil
		}
		return types.ExternalChain{}, false, err
	}
	return c, true, nil
}

func (k Keeper) CountChains(ctx context.Context) (uint32, error) { return mapCount(ctx, k.Chains) }

// ---- assets ----------------------------------------------------------------

func (k Keeper) GetAsset(ctx context.Context, id uint64) (types.Asset, bool, error) {
	a, err := k.Assets.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Asset{}, false, nil
		}
		return types.Asset{}, false, err
	}
	return a, true, nil
}

func (k Keeper) CountAssets(ctx context.Context) (uint32, error) { return mapCount(ctx, k.Assets) }

// ---- transfers -------------------------------------------------------------

func (k Keeper) GetOutbound(ctx context.Context, nonce uint64) (types.Outbound, bool, error) {
	o, err := k.Outbounds.Get(ctx, nonce)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Outbound{}, false, nil
		}
		return types.Outbound{}, false, err
	}
	return o, true, nil
}

func (k Keeper) GetInbound(ctx context.Context, id uint64) (types.Inbound, bool, error) {
	in, err := k.Inbounds.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Inbound{}, false, nil
		}
		return types.Inbound{}, false, err
	}
	return in, true, nil
}

// ---- settlement plumbing ---------------------------------------------------

func (k Keeper) requireSettlement() error {
	if k.settlement == nil {
		return types.ErrSettlement.Wrap("settlement keeper not wired")
	}
	return nil
}

// bridgeMint issues the native stablecoin 1:1 on an inbound release. The
// reserve gate, compliance checks, and supply accounting all live in
// x/stableusd's BridgeMint; here we only verify the denom is wired and known.
func (k Keeper) bridgeMint(ctx context.Context, denom, recipient string, amount uint64) error {
	if err := k.requireSettlement(); err != nil {
		return err
	}
	if amount == 0 {
		return nil
	}
	if !k.settlement.HasDenom(ctx, denom) {
		return types.ErrSettlement.Wrapf("unknown settlement denom %q", denom)
	}
	return k.settlement.BridgeMint(ctx, denom, recipient, amount)
}

// bridgeBurn destroys the holder's native stablecoin 1:1 on an outbound lock.
// Burn is allowed on a PAUSED (wind-down) denom; x/stableusd's BridgeBurn
// enforces holder compliance and sufficient balance.
func (k Keeper) bridgeBurn(ctx context.Context, denom, holder string, amount uint64) error {
	if err := k.requireSettlement(); err != nil {
		return err
	}
	if amount == 0 {
		return nil
	}
	if !k.settlement.HasDenom(ctx, denom) {
		return types.ErrSettlement.Wrapf("unknown settlement denom %q", denom)
	}
	return k.settlement.BridgeBurn(ctx, denom, holder, amount)
}

// EscrowBalance reports the net bridged outstanding for a denom: total minted
// via released inbounds minus total burned via outbounds (floored at 0). In the
// mint/burn model there is no escrow pool — this is the bridge-issued
// circulating amount. Backed by the MintedTotal/BurnedTotal counters kept in
// lockstep with Release/Lock (rebuilt from history at InitGenesis), so the
// read is O(1) instead of walking the full transfer history.
func (k Keeper) EscrowBalance(ctx context.Context, denom string) uint64 {
	minted, err := k.MintedTotal.Get(ctx, denom)
	if err != nil {
		minted = 0
	}
	burned, err := k.BurnedTotal.Get(ctx, denom)
	if err != nil {
		burned = 0
	}
	if burned >= minted {
		return 0
	}
	return minted - burned
}

// recordRelease keeps the release-side derived state in sync: the
// (chain, released_at, id) window index and the per-denom minted counter.
func (k Keeper) recordRelease(ctx context.Context, in types.Inbound, denom string) error {
	if err := k.ReleasedByChain.Set(ctx,
		collections.Join3(in.SrcChainId, uint64(in.ReleasedAt), in.Id), in.Amount); err != nil {
		return err
	}
	cur, err := k.MintedTotal.Get(ctx, denom)
	if err != nil && !errIsNotFound(err) {
		return err
	}
	next, err := types.SafeAdd(cur, in.Amount)
	if err != nil {
		return err
	}
	return k.MintedTotal.Set(ctx, denom, next)
}

// recordBurn bumps the per-denom burned counter on an outbound lock.
func (k Keeper) recordBurn(ctx context.Context, denom string, amount uint64) error {
	cur, err := k.BurnedTotal.Get(ctx, denom)
	if err != nil && !errIsNotFound(err) {
		return err
	}
	next, err := types.SafeAdd(cur, amount)
	if err != nil {
		return err
	}
	return k.BurnedTotal.Set(ctx, denom, next)
}

// mintedInWindow sums the amount of already-RELEASED inbounds from chainID whose
// release timestamp is strictly after `since`. It walks the time-ordered
// ReleasedByChain index newest-first and stops at the window boundary, so the
// cost is proportional to the releases inside the window — not to the full
// (ever-growing) inbound history.
func (k Keeper) mintedInWindow(ctx context.Context, chainID uint64, since int64) (uint64, error) {
	var total uint64
	rng := collections.NewPrefixedTripleRangeReversed[uint64, uint64, uint64](chainID)
	err := k.ReleasedByChain.Walk(ctx, rng, func(key collections.Triple[uint64, uint64, uint64], amount uint64) (bool, error) {
		if int64(key.K2()) <= since {
			return true, nil // everything older is outside the window
		}
		s, err := types.SafeAdd(total, amount)
		if err != nil {
			return true, err
		}
		total = s
		return false, nil
	})
	return total, err
}

// CountValidAttestations returns how many of an inbound's recorded attestations
// are still members of the CURRENT attestor set. Quorum is always evaluated
// against this count so a stale signature from a since-removed attestor can
// never contribute to (or retroactively manufacture) a release.
func CountValidAttestations(attestations, attestors []string) uint32 {
	var n uint32
	for _, a := range attestations {
		if types.Contains(attestors, a) {
			n++
		}
	}
	return n
}

// nextSeq returns a 1-indexed monotonic id from a collections.Sequence.
func nextSeq(ctx context.Context, seq collections.Sequence) (uint64, error) {
	n, err := seq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

func errIsNotFound(err error) bool { return errors.Is(err, collections.ErrNotFound) }

func mapCount[K any, V any](ctx context.Context, m collections.Map[K, V]) (uint32, error) {
	var n uint32
	if err := m.Walk(ctx, nil, func(K, V) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}
