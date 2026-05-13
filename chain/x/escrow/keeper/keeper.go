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

	"energychain/x/escrow/types"
)

// EscrowPoolAccount is the deterministic 20-byte module-derived
// account that holds escrowed stablecoin and RWA balances while in
// flight. Funds debited from a depositor at Fund time are credited
// here, and Release / Refund / ArbiterRelease / ArbiterRefund
// debit from here back to the appropriate counterparty.
//
// Using authtypes.NewModuleAddress with a well-known string
// guarantees the address is the standard Cosmos length, derived
// deterministically from the seed, and independent of any chain's
// bech32 prefix configuration. Both stablecoin and RWA holdings
// use this single pool — internal accounting is per-(token, escrow)
// in the Escrow row, so a shared pool is safe and minimises
// surface area.
var EscrowPoolAccount = authtypes.NewModuleAddress("escrow_pool").String()

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	sanctions  types.SanctionsKeeper
	stablecoin types.StablecoinKeeper
	rwa        types.RWAKeeper
	oracle     types.OracleKeeper
	audit      types.AuditKeeper

	Schema collections.Schema

	Params  collections.Item[types.Params]
	Escrows collections.Map[uint64, types.Escrow]
	IDSeq   collections.Sequence

	EscrowByDepositor   collections.KeySet[collections.Pair[string, uint64]]
	EscrowByBeneficiary collections.KeySet[collections.Pair[string, uint64]]

	Approvals collections.Map[collections.Pair[uint64, string], types.Approval]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	sanctions types.SanctionsKeeper,
	stablecoin types.StablecoinKeeper,
	rwa types.RWAKeeper,
	oracle types.OracleKeeper,
	audit types.AuditKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("escrow: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		sanctions:    sanctions,
		stablecoin:   stablecoin,
		rwa:          rwa,
		oracle:       oracle,
		audit:        audit,

		Params:  collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Escrows: collections.NewMap(sb, types.EscrowCollectionPrefix, "escrows", collections.Uint64Key, codec.CollValue[types.Escrow](cdc)),
		IDSeq:   collections.NewSequence(sb, types.EscrowIDSeqPrefix, "escrow_id_seq"),

		EscrowByDepositor: collections.NewKeySet(sb, types.EscrowByDepositorPrefix, "escrow_by_depositor",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		EscrowByBeneficiary: collections.NewKeySet(sb, types.EscrowByBeneficiaryPrefix, "escrow_by_beneficiary",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		Approvals: collections.NewMap(sb, types.ApprovalCollectionPrefix, "approvals",
			collections.PairKeyCodec(collections.Uint64Key, collections.StringKey),
			codec.CollValue[types.Approval](cdc)),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string                 { return k.authority }
func (k Keeper) SanctionsHook() types.SanctionsKeeper { return k.sanctions }

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

// ---- escrow CRUD ---------------------------------------------------------

func (k Keeper) GetEscrow(ctx context.Context, id uint64) (types.Escrow, bool, error) {
	e, err := k.Escrows.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Escrow{}, false, nil
		}
		return types.Escrow{}, false, err
	}
	return e, true, nil
}

func (k Keeper) MustGetEscrow(ctx context.Context, id uint64) (types.Escrow, error) {
	e, ok, err := k.GetEscrow(ctx, id)
	if err != nil {
		return types.Escrow{}, err
	}
	if !ok {
		return types.Escrow{}, fmt.Errorf("escrow %d not found", id)
	}
	return e, nil
}

func (k Keeper) SetEscrow(ctx context.Context, e types.Escrow) error {
	return k.Escrows.Set(ctx, e.Id, e)
}

func (k Keeper) NextEscrowID(ctx context.Context) (uint64, error) {
	n, err := k.IDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

// ---- escrow count cap ----------------------------------------------------

// CountEscrows is the O(1) bound enforced at CreateEscrow against
// Params.MaxEscrows. We rely on the IDSeq monotonic invariant —
// rows are never deleted — so peek == lifetime-create-count. If a
// future MsgArchiveEscrow is added, this method MUST be replaced
// by a maintained counter.
func (k Keeper) CountEscrows(ctx context.Context) (uint32, error) {
	v, err := k.IDSeq.Peek(ctx)
	if err != nil {
		return 0, err
	}
	if v > uint64(^uint32(0)) {
		return ^uint32(0), nil
	}
	return uint32(v), nil
}

// ---- approvals -----------------------------------------------------------

func (k Keeper) GetApproval(ctx context.Context, escrowID uint64, signer string) (types.Approval, bool, error) {
	a, err := k.Approvals.Get(ctx, collections.Join(escrowID, signer))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Approval{}, false, nil
		}
		return types.Approval{}, false, err
	}
	return a, true, nil
}

func (k Keeper) SetApproval(ctx context.Context, a types.Approval) error {
	return k.Approvals.Set(ctx, collections.Join(a.EscrowId, a.Signer), a)
}

func (k Keeper) RemoveApproval(ctx context.Context, escrowID uint64, signer string) error {
	return k.Approvals.Remove(ctx, collections.Join(escrowID, signer))
}

// IsSigner checks if the given address is on the escrow's committee.
// Linear in committee size; bounded by Params.MaxSignersPerEscrow.
func (Keeper) IsSigner(e types.Escrow, addr string) bool {
	for _, s := range e.Committee {
		if s == addr {
			return true
		}
	}
	return false
}

// ---- audit helper --------------------------------------------------------

func (k Keeper) recordAudit(ctx context.Context, escrowID uint64, action, actor, subject, detail string) {
	if k.audit == nil {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.audit.RecordEscrowAction(sdkCtx, escrowID, action, actor, subject, detail)
}

// ---- sanctions helper ----------------------------------------------------

// requireNotSanctioned is the address-only blacklist gate. When the
// require_sanctions_clear param is on (default), every payee must
// be unblocked at the time of the action; this is a re-check rather
// than a one-shot guarantee, so a sanction added between Fund and
// Release will block Release at the latest moment.
func (k Keeper) requireNotSanctioned(ctx context.Context, p types.Params, field, addr string) error {
	if !p.RequireSanctionsClear || k.sanctions == nil || addr == "" {
		return nil
	}
	if k.sanctions.IsSanctioned(sdk.UnwrapSDKContext(ctx), addr) {
		return fmt.Errorf("%s %s is sanctioned", field, addr)
	}
	return nil
}

// ---- oracle gate ---------------------------------------------------------

// checkOracleTrigger evaluates the optional oracle precondition for
// release. Returns nil when no oracle is configured or the value is
// fresh enough and >= the configured floor. Refusing the action is
// the safe default when the oracle keeper is unwired.
func (k Keeper) checkOracleTrigger(ctx context.Context, t types.Triggers, blockTime int64) error {
	if t.OracleTopicId == "" {
		return nil
	}
	if k.oracle == nil {
		return fmt.Errorf("oracle gate required for topic %q but oracle keeper not wired", t.OracleTopicId)
	}
	v, ts, ok := k.oracle.GetAggregatedReserve(sdk.UnwrapSDKContext(ctx), t.OracleTopicId)
	if !ok {
		return fmt.Errorf("oracle value missing for topic %q", t.OracleTopicId)
	}
	if v < t.OracleMinValue {
		return fmt.Errorf("oracle value %d below floor %d for topic %q", v, t.OracleMinValue, t.OracleTopicId)
	}
	if t.OracleMaxStalenessSeconds > 0 {
		age := blockTime - ts
		if age > t.OracleMaxStalenessSeconds {
			return fmt.Errorf("oracle value for %q stale (%ds > %ds)", t.OracleTopicId, age, t.OracleMaxStalenessSeconds)
		}
	}
	return nil
}

// ---- asset moves ---------------------------------------------------------

// payIn debits the depositor and credits the EscrowPoolAccount for
// the escrow's asset. Caller MUST verify Status == DRAFT and that
// the depositor is not sanctioned at the chain level. payIn
// additionally re-checks the denom-level (stablecoin) gates that
// MoveBalance bypasses: paused / retired denom and frozen /
// blacklisted depositor account. RWA-side gates (transferable +
// status) live inside x/rwa.EscrowLock.
func (k Keeper) payIn(ctx context.Context, e types.Escrow) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	switch e.Kind {
	case types.AssetKind_ASSET_KIND_STABLECOIN:
		if k.stablecoin == nil {
			return fmt.Errorf("stablecoin keeper not wired")
		}
		if !k.stablecoin.HasDenom(sdkCtx, e.StablecoinDenom) {
			return fmt.Errorf("denom %q not registered", e.StablecoinDenom)
		}
		if k.stablecoin.IsDenomPaused(sdkCtx, e.StablecoinDenom) {
			return fmt.Errorf("denom %q paused at the stablecoin layer; pay-in refused", e.StablecoinDenom)
		}
		if k.stablecoin.IsAccountBlocked(sdkCtx, e.StablecoinDenom, e.Depositor) {
			return fmt.Errorf("depositor %s is frozen / blacklisted on denom %s", e.Depositor, e.StablecoinDenom)
		}
		return k.stablecoin.Move(sdkCtx, e.StablecoinDenom, e.Depositor, EscrowPoolAccount, e.Amount)
	case types.AssetKind_ASSET_KIND_RWA:
		if k.rwa == nil {
			return fmt.Errorf("rwa keeper not wired")
		}
		if !k.rwa.HasToken(sdkCtx, e.RwaTokenId) {
			return fmt.Errorf("rwa token %d not found", e.RwaTokenId)
		}
		return k.rwa.EscrowLock(sdkCtx, e.RwaTokenId, e.Depositor, e.Amount)
	}
	return fmt.Errorf("unsupported asset kind %v", e.Kind)
}

// payOut debits the EscrowPoolAccount and credits `to` for the
// escrow's asset. Caller MUST verify chain-level sanctions on `to`
// and that the escrow is in a state allowing payout. payOut also
// re-checks the per-denom freeze on `to` so a stablecoin freeze
// added between Fund and Release blocks the payout (the pool /
// in-flight escrow becomes a refund candidate instead). The
// pool itself is exempt from the freeze check — it's a module
// account, not a holder.
func (k Keeper) payOut(ctx context.Context, e types.Escrow, to string) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	switch e.Kind {
	case types.AssetKind_ASSET_KIND_STABLECOIN:
		if k.stablecoin == nil {
			return fmt.Errorf("stablecoin keeper not wired")
		}
		if k.stablecoin.IsAccountBlocked(sdkCtx, e.StablecoinDenom, to) {
			return fmt.Errorf("recipient %s is frozen / blacklisted on denom %s", to, e.StablecoinDenom)
		}
		return k.stablecoin.Move(sdkCtx, e.StablecoinDenom, EscrowPoolAccount, to, e.Amount)
	case types.AssetKind_ASSET_KIND_RWA:
		if k.rwa == nil {
			return fmt.Errorf("rwa keeper not wired")
		}
		return k.rwa.EscrowRelease(sdkCtx, e.RwaTokenId, to, e.Amount)
	}
	return fmt.Errorf("unsupported asset kind %v", e.Kind)
}
