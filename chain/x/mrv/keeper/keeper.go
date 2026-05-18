package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/mrv/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	sanctions types.SanctionsKeeper
	did       types.DIDKeeper
	audit     types.AuditKeeper

	Schema collections.Schema

	Params collections.Item[types.Params]

	// Schemas keyed by id.
	Schemas       collections.Map[uint64, types.Schema]
	SchemaIDSeq   collections.Sequence
	// SchemaByAsset is a covering index for asset-class queries.
	SchemaByAsset collections.KeySet[collections.Pair[uint32, uint64]]

	// Verifiers keyed by id.
	Verifiers       collections.Map[uint64, types.Verifier]
	VerifierIDSeq   collections.Sequence
	// VerifierByDID maps did → id; uniqueness enforced.
	VerifierByDID   collections.Map[string, uint64]
	// VerifierBySigner maps chain signer_address → id;
	// uniqueness enforced so an address can back at most
	// one verifier.
	VerifierBySigner collections.Map[string, uint64]

	// Reports keyed by id, with subject + schema covering indexes.
	Reports             collections.Map[uint64, types.Report]
	ReportIDSeq         collections.Sequence
	ReportBySubject     collections.KeySet[collections.Pair[string, uint64]]
	ReportBySchema      collections.KeySet[collections.Pair[uint64, uint64]]
	ReportCountBySubject collections.Map[string, uint64]

	// View-key grants keyed by id, with granter / grantee / expiry
	// covering indexes. The expiry index drives the end-block
	// sweep that flips expired grants to revoked.
	Grants          collections.Map[uint64, types.ViewKeyGrant]
	GrantIDSeq      collections.Sequence
	GrantByGranter  collections.KeySet[collections.Pair[string, uint64]]
	GrantByGrantee  collections.KeySet[collections.Pair[string, uint64]]
	GrantByExpiry   collections.KeySet[collections.Pair[int64, uint64]]
}

// NewKeeper wires the persistent collections. did, sanctions
// and audit are all optional; nil disables the corresponding
// hook (used in tests and minimal-deployment chains).
func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	sanctions types.SanctionsKeeper,
	did types.DIDKeeper,
	audit types.AuditKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("mrv: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		cdc: cdc, storeService: storeService, authority: authority,
		sanctions: sanctions, did: did, audit: audit,

		Params: collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),

		Schemas:     collections.NewMap(sb, types.SchemaCollectionPrefix, "schemas", collections.Uint64Key, codec.CollValue[types.Schema](cdc)),
		SchemaIDSeq: collections.NewSequence(sb, types.SchemaIDSeqPrefix, "schema_id_seq"),
		SchemaByAsset: collections.NewKeySet(sb, types.SchemaByAssetPrefix, "schema_by_asset",
			collections.PairKeyCodec(collections.Uint32Key, collections.Uint64Key)),

		Verifiers:     collections.NewMap(sb, types.VerifierCollectionPrefix, "verifiers", collections.Uint64Key, codec.CollValue[types.Verifier](cdc)),
		VerifierIDSeq: collections.NewSequence(sb, types.VerifierIDSeqPrefix, "verifier_id_seq"),
		VerifierByDID: collections.NewMap(sb, types.VerifierByDIDPrefix, "verifier_by_did",
			collections.StringKey, collections.Uint64Value),
		VerifierBySigner: collections.NewMap(sb, types.VerifierBySignerPrefix, "verifier_by_signer",
			collections.StringKey, collections.Uint64Value),

		Reports:     collections.NewMap(sb, types.ReportCollectionPrefix, "reports", collections.Uint64Key, codec.CollValue[types.Report](cdc)),
		ReportIDSeq: collections.NewSequence(sb, types.ReportIDSeqPrefix, "report_id_seq"),
		ReportBySubject: collections.NewKeySet(sb, types.ReportBySubjectPrefix, "report_by_subject",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		ReportBySchema: collections.NewKeySet(sb, types.ReportBySchemaPrefix, "report_by_schema",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
		ReportCountBySubject: collections.NewMap(sb, types.ReportCountBySubjectPrefix, "report_count_by_subject",
			collections.StringKey, collections.Uint64Value),

		Grants:     collections.NewMap(sb, types.GrantCollectionPrefix, "grants", collections.Uint64Key, codec.CollValue[types.ViewKeyGrant](cdc)),
		GrantIDSeq: collections.NewSequence(sb, types.GrantIDSeqPrefix, "grant_id_seq"),
		GrantByGranter: collections.NewKeySet(sb, types.GrantByGranterPrefix, "grant_by_granter",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		GrantByGrantee: collections.NewKeySet(sb, types.GrantByGranteePrefix, "grant_by_grantee",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		GrantByExpiry: collections.NewKeySet(sb, types.GrantByExpiryPrefix, "grant_by_expiry",
			collections.PairKeyCodec(collections.Int64Key, collections.Uint64Key)),
	}
	sch, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = sch
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ---- Params ------------------------------------------------------------

func (k Keeper) SetParams(ctx context.Context, p types.Params) error { return k.Params.Set(ctx, p) }
func (k Keeper) GetParams(ctx context.Context) (types.Params, error) { return k.Params.Get(ctx) }

// ---- Sequences ---------------------------------------------------------

func (k Keeper) NextSchemaID(ctx context.Context) (uint64, error) {
	return k.next(ctx, k.SchemaIDSeq)
}
func (k Keeper) NextVerifierID(ctx context.Context) (uint64, error) {
	return k.next(ctx, k.VerifierIDSeq)
}
func (k Keeper) NextReportID(ctx context.Context) (uint64, error) {
	return k.next(ctx, k.ReportIDSeq)
}
func (k Keeper) NextGrantID(ctx context.Context) (uint64, error) {
	return k.next(ctx, k.GrantIDSeq)
}
func (k Keeper) next(ctx context.Context, seq collections.Sequence) (uint64, error) {
	v, err := seq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return v + 1, nil
}

// ---- Schemas -----------------------------------------------------------

func (k Keeper) SetSchema(ctx context.Context, s types.Schema) error {
	if err := k.Schemas.Set(ctx, s.Id, s); err != nil {
		return err
	}
	return k.SchemaByAsset.Set(ctx, collections.Join(uint32(s.AssetClass), s.Id))
}
func (k Keeper) GetSchema(ctx context.Context, id uint64) (types.Schema, bool, error) {
	v, err := k.Schemas.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Schema{}, false, nil
		}
		return types.Schema{}, false, err
	}
	return v, true, nil
}
func (k Keeper) MustGetSchema(ctx context.Context, id uint64) (types.Schema, error) {
	v, ok, err := k.GetSchema(ctx, id)
	if err != nil {
		return v, err
	}
	if !ok {
		return v, fmt.Errorf("schema %d not found", id)
	}
	return v, nil
}

// ---- Verifiers ---------------------------------------------------------

func (k Keeper) SetVerifier(ctx context.Context, v types.Verifier) error {
	return k.Verifiers.Set(ctx, v.Id, v)
}
func (k Keeper) GetVerifier(ctx context.Context, id uint64) (types.Verifier, bool, error) {
	v, err := k.Verifiers.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Verifier{}, false, nil
		}
		return types.Verifier{}, false, err
	}
	return v, true, nil
}
func (k Keeper) GetVerifierByDID(ctx context.Context, did string) (uint64, bool, error) {
	v, err := k.VerifierByDID.Get(ctx, did)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return v, true, nil
}
func (k Keeper) GetVerifierBySigner(ctx context.Context, addr string) (uint64, bool, error) {
	v, err := k.VerifierBySigner.Get(ctx, addr)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return v, true, nil
}

// ---- Reports -----------------------------------------------------------

func (k Keeper) SetReport(ctx context.Context, r types.Report) error {
	return k.Reports.Set(ctx, r.Id, r)
}
func (k Keeper) GetReport(ctx context.Context, id uint64) (types.Report, bool, error) {
	v, err := k.Reports.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.Report{}, false, nil
		}
		return types.Report{}, false, err
	}
	return v, true, nil
}
func (k Keeper) MustGetReport(ctx context.Context, id uint64) (types.Report, error) {
	v, ok, err := k.GetReport(ctx, id)
	if err != nil {
		return v, err
	}
	if !ok {
		return v, fmt.Errorf("report %d not found", id)
	}
	return v, nil
}

func (k Keeper) ReportCount(ctx context.Context, subject string) (uint64, error) {
	v, err := k.ReportCountBySubject.Get(ctx, subject)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

func (k Keeper) incReportCount(ctx context.Context, subject string) (uint64, error) {
	cur, err := k.ReportCount(ctx, subject)
	if err != nil {
		return 0, err
	}
	nb, err := types.SafeAdd(cur, 1)
	if err != nil {
		return 0, err
	}
	return nb, k.ReportCountBySubject.Set(ctx, subject, nb)
}

// ---- Grants ------------------------------------------------------------

func (k Keeper) SetGrant(ctx context.Context, g types.ViewKeyGrant) error {
	return k.Grants.Set(ctx, g.Id, g)
}
func (k Keeper) GetGrant(ctx context.Context, id uint64) (types.ViewKeyGrant, bool, error) {
	v, err := k.Grants.Get(ctx, id)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.ViewKeyGrant{}, false, nil
		}
		return types.ViewKeyGrant{}, false, err
	}
	return v, true, nil
}
func (k Keeper) MustGetGrant(ctx context.Context, id uint64) (types.ViewKeyGrant, error) {
	v, ok, err := k.GetGrant(ctx, id)
	if err != nil {
		return v, err
	}
	if !ok {
		return v, fmt.Errorf("grant %d not found", id)
	}
	return v, nil
}

// ---- Compliance hooks -------------------------------------------------

// requireUnsanctioned is a fail-closed check: if a sanctions
// keeper is wired, the address must not be sanctioned. nil
// keeper is the explicit opt-out for deployments that don't
// run a sanctions module.
func (k Keeper) requireUnsanctioned(ctx context.Context, who string) error {
	if k.sanctions == nil {
		return nil
	}
	if k.sanctions.IsSanctioned(sdk.UnwrapSDKContext(ctx), who) {
		return fmt.Errorf("address %s is sanctioned", who)
	}
	return nil
}

// recordAudit fires the optional audit hook. Failures inside
// the audit module MUST NOT roll back the calling tx — the
// audit module is best-effort observability, not a critical
// path.
func (k Keeper) recordAudit(ctx context.Context, reportID uint64, action, actor, subject, detail string) {
	if k.audit == nil {
		return
	}
	k.audit.RecordMRVAction(sdk.UnwrapSDKContext(ctx), reportID, action, actor, subject, detail)
}
