package keeper

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/carbon/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	policy    types.PolicyKeeper
	sanctions types.SanctionsKeeper
	oracle    types.OracleKeeper
	eac       types.EACKeeper
	audit     types.AuditKeeper
	// erc20 is the optional auto-registration hook into the
	// Cosmos EVM x/erc20 module. RegisterIssuer reserves a
	// TokenPair entry for the synthetic denom
	// "carbon"+issuer_id so EVM tooling can discover the
	// issuer's allowance / offset namespace.
	erc20 types.ERC20Keeper

	Schema collections.Schema

	Params  collections.Item[types.Params]
	Issuers collections.Map[string, types.Issuer]

	Assets         collections.Map[uint64, types.Asset]
	AssetByIssuer  collections.KeySet[collections.Pair[string, uint64]]
	AssetIDSeq     collections.Sequence

	Balances       collections.Map[collections.Pair[uint64, string], uint64]
	BalanceByOwner collections.KeySet[collections.Pair[string, uint64]]

	Retirements             collections.Map[uint64, types.Retirement]
	RetirementByBeneficiary collections.KeySet[collections.Pair[string, uint64]]
	RetirementByAsset       collections.KeySet[collections.Pair[uint64, uint64]]
	RetirementIDSeq         collections.Sequence

	Article6Authorizations collections.Map[uint64, types.Article6Authorization]

	Bridges collections.Map[uint64, types.BridgeAttestation]

	SourceSerialIndex collections.Map[collections.Triple[int32, string, string], uint64]
	EACOffsetClaimed  collections.Map[uint64, uint64]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	policy types.PolicyKeeper,
	sanctions types.SanctionsKeeper,
	oracle types.OracleKeeper,
	eac types.EACKeeper,
	audit types.AuditKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("carbon: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		policy:       policy,
		sanctions:    sanctions,
		oracle:       oracle,
		eac:          eac,
		audit:        audit,

		Params:  collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Issuers: collections.NewMap(sb, types.IssuerCollectionPrefix, "issuers", collections.StringKey, codec.CollValue[types.Issuer](cdc)),

		Assets:        collections.NewMap(sb, types.AssetCollectionPrefix, "assets", collections.Uint64Key, codec.CollValue[types.Asset](cdc)),
		AssetByIssuer: collections.NewKeySet(sb, types.AssetByIssuerPrefix, "asset_by_issuer", collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		AssetIDSeq:    collections.NewSequence(sb, types.AssetIDSeqPrefix, "asset_id_seq"),

		Balances:       collections.NewMap(sb, types.BalanceCollectionPrefix, "balances", collections.PairKeyCodec(collections.Uint64Key, collections.StringKey), collections.Uint64Value),
		BalanceByOwner: collections.NewKeySet(sb, types.BalanceByOwnerPrefix, "balance_by_owner", collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		Retirements:             collections.NewMap(sb, types.RetirementCollectionPrefix, "retirements", collections.Uint64Key, codec.CollValue[types.Retirement](cdc)),
		RetirementByBeneficiary: collections.NewKeySet(sb, types.RetirementByBeneficiaryPrefix, "retire_by_ben", collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		RetirementByAsset:       collections.NewKeySet(sb, types.RetirementByAssetPrefix, "retire_by_asset", collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
		RetirementIDSeq:         collections.NewSequence(sb, types.RetirementIDSeqPrefix, "retirement_id_seq"),

		Article6Authorizations: collections.NewMap(sb, types.Article6AuthorizationPrefix, "article6_auth", collections.Uint64Key, codec.CollValue[types.Article6Authorization](cdc)),

		Bridges: collections.NewMap(sb, types.BridgeCollectionPrefix, "bridges", collections.Uint64Key, codec.CollValue[types.BridgeAttestation](cdc)),

		SourceSerialIndex: collections.NewMap(sb, types.SourceSerialIndexPrefix, "source_serial_idx",
			collections.TripleKeyCodec(collections.Int32Key, collections.StringKey, collections.StringKey),
			collections.Uint64Value),
		EACOffsetClaimed: collections.NewMap(sb, types.EACOffsetClaimPrefix, "eac_offset_claim",
			collections.Uint64Key, collections.Uint64Value),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// WithERC20Keeper returns a Keeper value with the optional ERC20
// auto-registration hook attached. See the EAC equivalent for the
// lifetime rules (Keeper is a value type — reassign before any
// MsgServer or precompile is constructed against the keeper).
func (k Keeper) WithERC20Keeper(erc20 types.ERC20Keeper) Keeper {
	k.erc20 = erc20
	return k
}
func (k Keeper) SanctionsHook() types.SanctionsKeeper { return k.sanctions }
func (k Keeper) EACHook() types.EACKeeper             { return k.eac }

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

// ---- issuer ---------------------------------------------------------------

func (k Keeper) CountIssuers(ctx context.Context) (uint32, error) {
	var n uint32
	if err := k.Issuers.Walk(ctx, nil, func(string, types.Issuer) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// ---- asset id seq ---------------------------------------------------------

func (k Keeper) NextAssetID(ctx context.Context) (uint64, error) {
	id, err := k.AssetIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return id + 1, nil
}

// ---- balance plumbing -----------------------------------------------------

func (k Keeper) GetBalance(ctx context.Context, assetID uint64, holder string) (uint64, error) {
	v, err := k.Balances.Get(ctx, collections.Join(assetID, holder))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

func (k Keeper) setBalance(ctx context.Context, assetID uint64, holder string, amt uint64) error {
	pk := collections.Join(assetID, holder)
	ok := collections.Join(holder, assetID)
	if amt == 0 {
		if err := k.Balances.Remove(ctx, pk); err != nil {
			return err
		}
		return k.BalanceByOwner.Remove(ctx, ok)
	}
	if err := k.Balances.Set(ctx, pk, amt); err != nil {
		return err
	}
	return k.BalanceByOwner.Set(ctx, ok)
}

func (k Keeper) creditBalance(ctx context.Context, assetID uint64, holder string, amt uint64) error {
	if amt == 0 {
		return nil
	}
	cur, err := k.GetBalance(ctx, assetID, holder)
	if err != nil {
		return err
	}
	next, err := types.SafeAdd(cur, amt)
	if err != nil {
		return err
	}
	return k.setBalance(ctx, assetID, holder, next)
}

func (k Keeper) debitBalance(ctx context.Context, assetID uint64, holder string, amt uint64) error {
	if amt == 0 {
		return nil
	}
	cur, err := k.GetBalance(ctx, assetID, holder)
	if err != nil {
		return err
	}
	next, err := types.SafeSub(cur, amt)
	if err != nil {
		return fmt.Errorf("balance underflow for asset %d holder %s: %w", assetID, holder, err)
	}
	return k.setBalance(ctx, assetID, holder, next)
}

func (k Keeper) moveBalance(ctx context.Context, assetID uint64, from, to string, amt uint64) error {
	if err := k.debitBalance(ctx, assetID, from, amt); err != nil {
		return err
	}
	return k.creditBalance(ctx, assetID, to, amt)
}

// ---- compliance pipeline -------------------------------------------------

// transferCompliance is the unified gate for transfers and retirements.
// Order: address-only sanctions check (cheap) → policy DSL.
func (k Keeper) transferCompliance(ctx context.Context, assetID uint64, sender, receiver string, amount uint64) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	if params.RequireSanctionsClear && k.sanctions != nil {
		if sender != "" && k.sanctions.IsSanctioned(sdkCtx, sender) {
			return fmt.Errorf("sender %s is on the sanctions list", sender)
		}
		if receiver != "" && k.sanctions.IsSanctioned(sdkCtx, receiver) {
			return fmt.Errorf("receiver %s is on the sanctions list", receiver)
		}
	}
	if k.policy != nil {
		assetIDStr := strconv.FormatUint(assetID, 10)
		if err := k.policy.EvaluateTransfer(sdkCtx, "carbon", assetIDStr, sender, receiver, amount); err != nil {
			return fmt.Errorf("policy denied: %w", err)
		}
	}
	return nil
}

// ---- bridge --------------------------------------------------------------

func (k Keeper) CheckBridgeMintCoverage(ctx context.Context, asset types.Asset, mintUnits uint64) error {
	br, err := k.Bridges.Get(ctx, asset.Id)
	if err != nil {
		return fmt.Errorf("asset %d has no bridge attestation", asset.Id)
	}
	if k.oracle == nil {
		return fmt.Errorf("oracle keeper not wired; cannot verify bridge attestation")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	value, ts, ok := k.oracle.GetAggregatedReserve(sdkCtx, br.OracleTopicId)
	if !ok {
		return fmt.Errorf("no attestation for topic %s", br.OracleTopicId)
	}
	now := sdkCtx.BlockTime().Unix()
	if br.MaxStalenessSeconds > 0 && now-ts > int64(br.MaxStalenessSeconds) {
		return fmt.Errorf("attestation for topic %s is stale (age=%ds, max=%ds)",
			br.OracleTopicId, now-ts, br.MaxStalenessSeconds)
	}
	if value < 0 {
		return fmt.Errorf("attestation value %d is negative", value)
	}
	live, err := types.SafeSub(asset.IssuedUnits, asset.RetiredUnits)
	if err != nil {
		return err
	}
	next, err := types.SafeAdd(live, mintUnits)
	if err != nil {
		return err
	}
	if next > uint64(value) {
		return fmt.Errorf("bridge mint exceeds attested locked units: live+mint=%d > attested=%d", next, value)
	}
	return nil
}

// ---- EAC mutual exclusion -----------------------------------------------

// reserveEACClaim adds `units` to the running tally of OFFSET units
// referencing `eacCertID` and refuses to push the tally past the
// EAC certificate's own IssuedUnits. Fail-closed when the EAC keeper
// isn't wired or the referenced certificate is missing — so a chain
// with no EAC module configured cannot accidentally bypass the rule
// just because the link field was set.
func (k Keeper) reserveEACClaim(ctx context.Context, eacCertID, units uint64) error {
	if eacCertID == 0 {
		return nil
	}
	if k.eac == nil {
		return fmt.Errorf("offset references EAC certificate %d but EAC keeper not wired", eacCertID)
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	cap, ok := k.eac.GetCertificateIssuedUnits(sdkCtx, eacCertID)
	if !ok {
		return fmt.Errorf("EAC certificate %d not found", eacCertID)
	}
	cur, err := k.GetEACClaimed(ctx, eacCertID)
	if err != nil {
		return err
	}
	next, err := types.SafeAdd(cur, units)
	if err != nil {
		return err
	}
	if next > cap {
		return fmt.Errorf("EAC %d mutual-exclusion cap: claimed %d + new %d > issued %d",
			eacCertID, cur, units, cap)
	}
	return k.EACOffsetClaimed.Set(ctx, eacCertID, next)
}

func (k Keeper) GetEACClaimed(ctx context.Context, eacCertID uint64) (uint64, error) {
	v, err := k.EACOffsetClaimed.Get(ctx, eacCertID)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

// ---- audit helper -------------------------------------------------------

func (k Keeper) recordAudit(ctx context.Context, assetID uint64, issuerID, action, actor, beneficiary, detail string) {
	if k.audit == nil {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.audit.RecordCarbonAction(sdkCtx, assetID, issuerID, action, actor, beneficiary, detail)
}

// ---- retirement counter --------------------------------------------------

func (k Keeper) NextRetirementID(ctx context.Context) (uint64, error) {
	id, err := k.RetirementIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return id + 1, nil
}

// ---- genesis -------------------------------------------------------------

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return err
	}
	for _, is := range gs.Issuers {
		if err := k.Issuers.Set(ctx, is.Id, is); err != nil {
			return err
		}
	}
	for _, a := range gs.Assets {
		if err := k.Assets.Set(ctx, a.Id, a); err != nil {
			return err
		}
		if err := k.AssetByIssuer.Set(ctx, collections.Join(a.IssuerId, a.Id)); err != nil {
			return err
		}
		if a.SourceSerial != "" {
			key := collections.Join3(int32(a.Category), a.SourceRegistry, a.SourceSerial)
			if has, err := k.SourceSerialIndex.Has(ctx, key); err != nil {
				return err
			} else if has {
				return fmt.Errorf("genesis: duplicate source serial (category=%s registry=%s serial=%s)",
					a.Category, a.SourceRegistry, a.SourceSerial)
			}
			if err := k.SourceSerialIndex.Set(ctx, key, a.Id); err != nil {
				return err
			}
		}
	}
	for _, b := range gs.Balances {
		if err := k.setBalance(ctx, b.AssetId, b.Account, b.Amount); err != nil {
			return err
		}
	}
	for _, r := range gs.Retirements {
		if err := k.Retirements.Set(ctx, r.Id, r); err != nil {
			return err
		}
		if err := k.RetirementByBeneficiary.Set(ctx, collections.Join(r.Beneficiary, r.Id)); err != nil {
			return err
		}
		if err := k.RetirementByAsset.Set(ctx, collections.Join(r.AssetId, r.Id)); err != nil {
			return err
		}
	}
	for _, auth := range gs.Authorizations {
		if err := k.Article6Authorizations.Set(ctx, auth.AssetId, auth); err != nil {
			return err
		}
	}
	for _, br := range gs.Bridges {
		if err := k.Bridges.Set(ctx, br.AssetId, br); err != nil {
			return err
		}
	}
	for _, c := range gs.EacClaims {
		if err := k.EACOffsetClaimed.Set(ctx, c.EacCertificateId, c.ClaimedUnits); err != nil {
			return err
		}
	}
	if gs.AssetIdSeq > 0 {
		if err := k.AssetIDSeq.Set(ctx, gs.AssetIdSeq); err != nil {
			return err
		}
	}
	if gs.RetirementIdSeq > 0 {
		if err := k.RetirementIDSeq.Set(ctx, gs.RetirementIdSeq); err != nil {
			return err
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	params, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	gs := &types.GenesisState{Params: params}
	if err := k.Issuers.Walk(ctx, nil, func(_ string, v types.Issuer) (bool, error) {
		gs.Issuers = append(gs.Issuers, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Assets.Walk(ctx, nil, func(_ uint64, v types.Asset) (bool, error) {
		gs.Assets = append(gs.Assets, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Balances.Walk(ctx, nil, func(key collections.Pair[uint64, string], v uint64) (bool, error) {
		gs.Balances = append(gs.Balances, types.Balance{
			AssetId: key.K1(), Account: key.K2(), Amount: v,
		})
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Retirements.Walk(ctx, nil, func(_ uint64, v types.Retirement) (bool, error) {
		gs.Retirements = append(gs.Retirements, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Article6Authorizations.Walk(ctx, nil, func(_ uint64, v types.Article6Authorization) (bool, error) {
		gs.Authorizations = append(gs.Authorizations, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Bridges.Walk(ctx, nil, func(_ uint64, v types.BridgeAttestation) (bool, error) {
		gs.Bridges = append(gs.Bridges, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.EACOffsetClaimed.Walk(ctx, nil, func(k uint64, v uint64) (bool, error) {
		gs.EacClaims = append(gs.EacClaims, types.EACClaim{
			EacCertificateId: k, ClaimedUnits: v,
		})
		return false, nil
	}); err != nil {
		return nil, err
	}
	aseq, err := k.AssetIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	rseq, err := k.RetirementIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.AssetIdSeq = aseq
	gs.RetirementIdSeq = rseq
	return gs, nil
}
