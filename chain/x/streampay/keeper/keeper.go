package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"energychain/x/streampay/types"
)

// streamPayPool is the deterministic 20-byte module address that
// custodies in-flight stream balances. Funds debited from a
// sender on CreateStream / DepositToStream are credited here;
// Withdraw / Cancel drain back to receiver / sender from this
// address. Computed once, exposed via PoolAddress() to keep a
// single source of truth.
var streamPayPool = authtypes.NewModuleAddress("streampay_pool").String()

// PoolAddress returns the canonical streampay pool address.
func PoolAddress() string { return streamPayPool }

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	sanctions  types.SanctionsKeeper
	stablecoin types.StablecoinKeeper
	audit      types.AuditKeeper

	Schema collections.Schema

	Params           collections.Item[types.Params]
	Streams          collections.Map[uint64, types.Stream]
	IDSeq            collections.Sequence
	StreamBySender   collections.KeySet[collections.Pair[string, uint64]]
	StreamByReceiver collections.KeySet[collections.Pair[string, uint64]]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	sanctions types.SanctionsKeeper,
	stablecoin types.StablecoinKeeper,
	audit types.AuditKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("streampay: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		sanctions:    sanctions,
		stablecoin:   stablecoin,
		audit:        audit,

		Params:  collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Streams: collections.NewMap(sb, types.StreamCollectionPrefix, "streams", collections.Uint64Key, codec.CollValue[types.Stream](cdc)),
		IDSeq:   collections.NewSequence(sb, types.StreamIDSeqPrefix, "stream_id_seq"),
		StreamBySender: collections.NewKeySet(sb, types.StreamBySenderPrefix, "stream_by_sender",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		StreamByReceiver: collections.NewKeySet(sb, types.StreamByReceiverPrefix, "stream_by_receiver",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
	}
	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ---- params --------------------------------------------------------------

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

// ---- stream CRUD ---------------------------------------------------------

func (k Keeper) GetStream(ctx context.Context, id uint64) (types.Stream, bool, error) {
	s, err := k.Streams.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Stream{}, false, nil
		}
		return types.Stream{}, false, err
	}
	return s, true, nil
}

func (k Keeper) MustGetStream(ctx context.Context, id uint64) (types.Stream, error) {
	s, ok, err := k.GetStream(ctx, id)
	if err != nil {
		return types.Stream{}, err
	}
	if !ok {
		return types.Stream{}, fmt.Errorf("stream %d not found", id)
	}
	return s, nil
}

func (k Keeper) SetStream(ctx context.Context, s types.Stream) error {
	return k.Streams.Set(ctx, s.Id, s)
}

func (k Keeper) NextStreamID(ctx context.Context) (uint64, error) {
	n, err := k.IDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

// CountStreams is the O(1) bound for the MaxStreams cap. Mirrors
// the IDSeq-peek pattern used in x/escrow and x/scheduler — rows
// are never deleted (audit), so peek == lifetime-creates.
func (k Keeper) CountStreams(ctx context.Context) (uint32, error) {
	v, err := k.IDSeq.Peek(ctx)
	if err != nil {
		return 0, err
	}
	if v > uint64(^uint32(0)) {
		return ^uint32(0), nil
	}
	return uint32(v), nil
}

// CountStreamsForSender walks the (sender, *) slice of the
// by-sender index. Bounded by Params.MaxStreamsPerSender per
// CreateStream, so worst case is acceptable per tx.
func (k Keeper) CountStreamsForSender(ctx context.Context, sender string) (uint32, error) {
	rng := collections.NewPrefixedPairRange[string, uint64](sender)
	var n uint32
	if err := k.StreamBySender.Walk(ctx, rng, func(_ collections.Pair[string, uint64]) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// ---- pool plumbing -------------------------------------------------------

// fundPool debits `from` and credits the pool. Re-checks the
// stablecoin-level gates that MoveBalance bypasses so a paused
// denom or frozen sender cannot launder through the stream
// module.
func (k Keeper) fundPool(ctx context.Context, denom, from string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.stablecoin == nil {
		return fmt.Errorf("stablecoin keeper not wired")
	}
	if !k.stablecoin.HasDenom(sdkCtx, denom) {
		return fmt.Errorf("denom %q not registered", denom)
	}
	if k.stablecoin.IsDenomPaused(sdkCtx, denom) {
		return fmt.Errorf("denom %q paused; pay-in refused", denom)
	}
	if k.stablecoin.IsAccountBlocked(sdkCtx, denom, from) {
		return fmt.Errorf("payer %s is frozen / blacklisted on denom %s", from, denom)
	}
	return k.stablecoin.Move(sdkCtx, denom, from, PoolAddress(), amount)
}

// drainPool debits the pool and credits `to`. Refuses payouts
// to frozen / blacklisted recipients; denom-paused state does
// NOT block payouts so an in-flight stream can wind down even
// after a denom-wide pause.
func (k Keeper) drainPool(ctx context.Context, denom, to string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.stablecoin == nil {
		return fmt.Errorf("stablecoin keeper not wired")
	}
	if k.stablecoin.IsAccountBlocked(sdkCtx, denom, to) {
		return fmt.Errorf("payee %s is frozen / blacklisted on denom %s", to, denom)
	}
	return k.stablecoin.Move(sdkCtx, denom, PoolAddress(), to, amount)
}

// requireUnsanctioned is the chain-level sanctions gate. Called
// at create + every settlement leg so a freshly-sanctioned party
// cannot continue draining or being paid by an in-flight stream.
func (k Keeper) requireUnsanctioned(ctx context.Context, who, role string) error {
	if k.sanctions == nil {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.sanctions.IsSanctioned(sdkCtx, who) {
		return fmt.Errorf("%s %s is sanctioned", role, who)
	}
	return nil
}

// ---- audit -------------------------------------------------------------

func (k Keeper) recordAudit(ctx context.Context, id uint64, action, actor, subject, detail string) {
	if k.audit == nil {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.audit.RecordStreamPayAction(sdkCtx, id, action, actor, subject, detail)
}
