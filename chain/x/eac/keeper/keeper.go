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

	"energychain/x/eac/types"
)

type Keeper struct {
	cdc          codec.BinaryCodec
	storeService store.KVStoreService
	authority    string

	// optional cross-module collaborators — nil-safe at call sites.
	policy    types.PolicyKeeper
	sanctions types.SanctionsKeeper
	oracle    types.OracleKeeper
	audit     types.AuditKeeper
	// erc20 is the optional auto-registration hook into the
	// Cosmos EVM x/erc20 module. When wired, RegisterIssuer
	// reserves a TokenPair entry for the issuer's synthetic
	// denom ("eac"+issuer_id) so EVM tooling can list every
	// issuer's certificates as a single ERC20 namespace.
	erc20 types.ERC20Keeper

	Schema collections.Schema

	Params      collections.Item[types.Params]
	Issuers     collections.Map[string, types.Issuer]
	Certificates collections.Map[uint64, types.Certificate]
	CertByIssuer collections.KeySet[collections.Pair[string, uint64]]
	CertIDSeq   collections.Sequence

	Balances     collections.Map[collections.Pair[uint64, string], uint64]
	BalanceByOwner collections.KeySet[collections.Pair[string, uint64]]

	Retirements             collections.Map[uint64, types.Retirement]
	RetirementByBeneficiary collections.KeySet[collections.Pair[string, uint64]]
	RetirementByCertificate collections.KeySet[collections.Pair[uint64, uint64]]
	RetirementIDSeq         collections.Sequence

	Bridges collections.Map[uint64, types.BridgeAttestation]

	// SourceSerialIndex pins (kind:int32, registry:string, serial:string)
	// to the certificate id that owns it. Used by IssueBatch to reject
	// double-issuance of the same upstream serial.
	SourceSerialIndex collections.Map[collections.Triple[int32, string, string], uint64]
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	authority string,
	policy types.PolicyKeeper,
	sanctions types.SanctionsKeeper,
	oracle types.OracleKeeper,
	audit types.AuditKeeper,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(fmt.Errorf("eac: invalid authority %q: %w", authority, err))
	}
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,
		policy:       policy,
		sanctions:    sanctions,
		oracle:       oracle,
		audit:        audit,

		Params:  collections.NewItem(sb, types.ParamsCollectionPrefix, "params", codec.CollValue[types.Params](cdc)),
		Issuers: collections.NewMap(sb, types.IssuerCollectionPrefix, "issuers", collections.StringKey, codec.CollValue[types.Issuer](cdc)),
		Certificates: collections.NewMap(sb, types.CertificateCollectionPrefix, "certificates", collections.Uint64Key, codec.CollValue[types.Certificate](cdc)),
		CertByIssuer: collections.NewKeySet(sb, types.CertByIssuerPrefix, "cert_by_issuer", collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		CertIDSeq:    collections.NewSequence(sb, types.CertIDSeqPrefix, "cert_id_seq"),

		Balances:       collections.NewMap(sb, types.BalanceCollectionPrefix, "balances", collections.PairKeyCodec(collections.Uint64Key, collections.StringKey), collections.Uint64Value),
		BalanceByOwner: collections.NewKeySet(sb, types.BalanceByOwnerPrefix, "balance_by_owner", collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),

		Retirements:             collections.NewMap(sb, types.RetirementCollectionPrefix, "retirements", collections.Uint64Key, codec.CollValue[types.Retirement](cdc)),
		RetirementByBeneficiary: collections.NewKeySet(sb, types.RetirementByBeneficiaryPrefix, "retire_by_ben", collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		RetirementByCertificate: collections.NewKeySet(sb, types.RetirementByCertificatePrefix, "retire_by_cert", collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
		RetirementIDSeq:         collections.NewSequence(sb, types.RetirementIDSeqPrefix, "retirement_id_seq"),

		Bridges: collections.NewMap(sb, types.BridgeCollectionPrefix, "bridges", collections.Uint64Key, codec.CollValue[types.BridgeAttestation](cdc)),

		SourceSerialIndex: collections.NewMap(sb, types.SourceSerialIndexPrefix, "source_serial_idx",
			collections.TripleKeyCodec(collections.Int32Key, collections.StringKey, collections.StringKey),
			collections.Uint64Value),
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
// auto-registration hook attached. Callers MUST reassign the result
// (Keeper is a value type) and MUST do so BEFORE constructing the
// MsgServer / precompile that wraps the keeper.
func (k Keeper) WithERC20Keeper(erc20 types.ERC20Keeper) Keeper {
	k.erc20 = erc20
	return k
}

// GetCertificateIssuedUnits is the cross-module read used by x/carbon
// to enforce the "1 MWh cannot be both RE-claimed and offset-claimed"
// rule. Returns the certificate's IssuedUnits and a boolean for
// existence; never mutates state.
func (k Keeper) GetCertificateIssuedUnits(ctx sdk.Context, certificateID uint64) (uint64, bool) {
	c, err := k.Certificates.Get(ctx, certificateID)
	if err != nil {
		return 0, false
	}
	return c.IssuedUnits, true
}

// SanctionsHook returns the bound x/sanctions adapter (or nil if not
// wired). Exposed so the msg_server can run the same fail-closed
// receiver gate at issuance / bridge-mint time, even though those
// flows do not go through the transferCompliance pipeline.
func (k Keeper) SanctionsHook() types.SanctionsKeeper { return k.sanctions }

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

// ---- certificate / balance plumbing --------------------------------------

func (k Keeper) NextCertID(ctx context.Context) (uint64, error) {
	id, err := k.CertIDSeq.Next(ctx)
	if err != nil {
		return 0, err
	}
	return id + 1, nil
}

func (k Keeper) GetBalance(ctx context.Context, certID uint64, holder string) (uint64, error) {
	v, err := k.Balances.Get(ctx, collections.Join(certID, holder))
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

// setBalance writes amt under the (cert, holder) key. amt == 0 prunes both
// the primary balance row and the secondary owner index, keeping state
// minimal and the genesis invariant easy to enforce.
func (k Keeper) setBalance(ctx context.Context, certID uint64, holder string, amt uint64) error {
	pk := collections.Join(certID, holder)
	ok := collections.Join(holder, certID)
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

func (k Keeper) creditBalance(ctx context.Context, certID uint64, holder string, amt uint64) error {
	if amt == 0 {
		return nil
	}
	cur, err := k.GetBalance(ctx, certID, holder)
	if err != nil {
		return err
	}
	next, err := types.SafeAdd(cur, amt)
	if err != nil {
		return err
	}
	return k.setBalance(ctx, certID, holder, next)
}

func (k Keeper) debitBalance(ctx context.Context, certID uint64, holder string, amt uint64) error {
	if amt == 0 {
		return nil
	}
	cur, err := k.GetBalance(ctx, certID, holder)
	if err != nil {
		return err
	}
	next, err := types.SafeSub(cur, amt)
	if err != nil {
		return fmt.Errorf("balance underflow for cert %d holder %s: %w", certID, holder, err)
	}
	return k.setBalance(ctx, certID, holder, next)
}

func (k Keeper) moveBalance(ctx context.Context, certID uint64, from, to string, amt uint64) error {
	if err := k.debitBalance(ctx, certID, from, amt); err != nil {
		return err
	}
	return k.creditBalance(ctx, certID, to, amt)
}

// ---- compliance pipeline ------------------------------------------------

// transferCompliance is the single funnel for every transfer-or-retire
// gate. It is fail-closed: any unexpected error from a sub-checker
// short-circuits the operation. Order matters: cheaper / address-only
// checks come first so we never call the (potentially expensive)
// policy DSL for an obviously-blocked counterparty.
func (k Keeper) transferCompliance(ctx context.Context, certID uint64, sender, receiver string, amount uint64) error {
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
		assetID := strconv.FormatUint(certID, 10)
		if err := k.policy.EvaluateTransfer(sdkCtx, "eac", assetID, sender, receiver, amount); err != nil {
			return fmt.Errorf("policy denied: %w", err)
		}
	}
	return nil
}

// ---- bridge --------------------------------------------------------------

// CheckBridgeMintCoverage ensures the on-chain mirror after the
// proposed mint will not exceed the locked count attested by the
// upstream registry (via the bound oracle topic). Fail-closed when
// the attestation is missing or stale.
func (k Keeper) CheckBridgeMintCoverage(ctx context.Context, cert types.Certificate, mintUnits uint64) error {
	br, err := k.Bridges.Get(ctx, cert.Id)
	if err != nil {
		return fmt.Errorf("certificate %d has no bridge attestation", cert.Id)
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
	// Live = issued - retired ; remaining mintable = attested - live
	live, err := types.SafeSub(cert.IssuedUnits, cert.RetiredUnits)
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

// ---- audit helper -------------------------------------------------------

func (k Keeper) recordAudit(ctx context.Context, certID uint64, issuerID, action, actor, beneficiary, detail string) {
	if k.audit == nil {
		return
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.audit.RecordEACAction(sdkCtx, certID, issuerID, action, actor, beneficiary, detail)
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
	for _, c := range gs.Certificates {
		if err := k.Certificates.Set(ctx, c.Id, c); err != nil {
			return err
		}
		if err := k.CertByIssuer.Set(ctx, collections.Join(c.IssuerId, c.Id)); err != nil {
			return err
		}
		if c.SourceSerial != "" {
			key := collections.Join3(int32(c.Kind), c.SourceRegistry, c.SourceSerial)
			if has, err := k.SourceSerialIndex.Has(ctx, key); err != nil {
				return err
			} else if has {
				return fmt.Errorf("genesis: duplicate source serial (kind=%s registry=%s serial=%s)",
					c.Kind, c.SourceRegistry, c.SourceSerial)
			}
			if err := k.SourceSerialIndex.Set(ctx, key, c.Id); err != nil {
				return err
			}
		}
	}
	for _, b := range gs.Balances {
		if err := k.setBalance(ctx, b.CertificateId, b.Account, b.Amount); err != nil {
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
		if err := k.RetirementByCertificate.Set(ctx, collections.Join(r.CertificateId, r.Id)); err != nil {
			return err
		}
	}
	for _, br := range gs.Bridges {
		if err := k.Bridges.Set(ctx, br.CertificateId, br); err != nil {
			return err
		}
	}
	if gs.CertificateIdSeq > 0 {
		if err := k.CertIDSeq.Set(ctx, gs.CertificateIdSeq); err != nil {
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
	if err := k.Certificates.Walk(ctx, nil, func(_ uint64, v types.Certificate) (bool, error) {
		gs.Certificates = append(gs.Certificates, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Balances.Walk(ctx, nil, func(key collections.Pair[uint64, string], v uint64) (bool, error) {
		gs.Balances = append(gs.Balances, types.Balance{
			CertificateId: key.K1(),
			Account:       key.K2(),
			Amount:        v,
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
	if err := k.Bridges.Walk(ctx, nil, func(_ uint64, v types.BridgeAttestation) (bool, error) {
		gs.Bridges = append(gs.Bridges, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	cseq, err := k.CertIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	rseq, err := k.RetirementIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.CertificateIdSeq = cseq
	gs.RetirementIdSeq = rseq
	return gs, nil
}
