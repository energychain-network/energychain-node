package keeper

import (
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	corestore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/did/types"
)

// Keeper holds the declarative collections backing the DID module's
// persistent state. Three logical sub-stores share the keeper:
//
//   - DID Documents (primary) + (controller, subject) index
//   - Verifiable Credential statuses (primary) + by-subject + by-issuer
//     + by-expiry indexes
//   - Trust Anchors
//
// The expiry index is populated on Issue and trimmed on Revoke / GC, so
// that an end-blocker can scan a small window per block (at most
// `params.RevocationGracePeriod` worth) instead of the entire credential
// store.
type Keeper struct {
	cdc          codec.Codec
	storeService corestore.KVStoreService
	authority    string

	Schema collections.Schema

	Params           collections.Item[types.Params]
	Documents        collections.Map[string, types.DIDDocument]
	ControllerIndex  collections.KeySet[collections.Pair[string, string]]
	Credentials      collections.Map[string, types.CredentialStatus]
	CredBySubject    collections.KeySet[collections.Pair[string, string]]
	CredByIssuer     collections.KeySet[collections.Pair[string, string]]
	CredByExpiry     collections.KeySet[collections.Pair[int64, string]]
	Anchors          collections.Map[string, types.TrustAnchor]
}

func NewKeeper(cdc codec.Codec, storeService corestore.KVStoreService, authority string) Keeper {
	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		cdc:          cdc,
		storeService: storeService,
		authority:    authority,

		Params: collections.NewItem(
			sb,
			types.ParamsCollectionPrefix,
			"params",
			codec.CollValue[types.Params](cdc),
		),
		Documents: collections.NewMap(
			sb,
			types.DIDDocumentCollectionPrefix,
			"documents",
			collections.StringKey,
			codec.CollValue[types.DIDDocument](cdc),
		),
		ControllerIndex: collections.NewKeySet(
			sb,
			types.ControllerIndexPrefix,
			"controller_index",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
		),
		Credentials: collections.NewMap(
			sb,
			types.CredentialCollectionPrefix,
			"credentials",
			collections.StringKey,
			codec.CollValue[types.CredentialStatus](cdc),
		),
		CredBySubject: collections.NewKeySet(
			sb,
			types.CredentialBySubjectIndexPrefix,
			"credential_by_subject",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
		),
		CredByIssuer: collections.NewKeySet(
			sb,
			types.CredentialByIssuerIndexPrefix,
			"credential_by_issuer",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
		),
		CredByExpiry: collections.NewKeySet(
			sb,
			types.CredentialByExpiryIndexPrefix,
			"credential_by_expiry",
			collections.PairKeyCodec(collections.Int64Key, collections.StringKey),
		),
		Anchors: collections.NewMap(
			sb,
			types.AnchorCollectionPrefix,
			"anchors",
			collections.StringKey,
			codec.CollValue[types.TrustAnchor](cdc),
		),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(fmt.Errorf("did keeper: build schema: %w", err))
	}
	k.Schema = schema
	return k
}

func (k Keeper) GetAuthority() string { return k.authority }

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := k.Params.Set(ctx, p); err != nil {
		return fmt.Errorf("set did params: %w", err)
	}
	return nil
}

func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	p, err := k.Params.Get(ctx)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("did params decode failed", "err", err)
		}
		return types.DefaultParams()
	}
	return p
}

// ---------------------------------------------------------------------------
// DID Documents
// ---------------------------------------------------------------------------

// SetDocument writes the document and refreshes its (controller, subject)
// index entries. It overwrites any existing entries — callers are
// responsible for first removing stale (controller, subject) pairs by
// calling clearControllerIndex when controllers change.
func (k Keeper) SetDocument(ctx sdk.Context, d types.DIDDocument) error {
	if err := k.Documents.Set(ctx, d.Id, d); err != nil {
		return fmt.Errorf("store did document %s: %w", d.Id, err)
	}
	for _, c := range d.Controllers {
		if err := k.ControllerIndex.Set(ctx, collections.Join(c, d.Id)); err != nil {
			return fmt.Errorf("index controller %s -> %s: %w", c, d.Id, err)
		}
	}
	return nil
}

// ClearControllerIndex removes every (controller, subject=id) entry for
// the listed controllers. Called by UpdateDID before re-indexing with the
// new controller set so that the index never carries stale entries.
func (k Keeper) ClearControllerIndex(ctx sdk.Context, controllers []string, subject string) error {
	for _, c := range controllers {
		if err := k.ControllerIndex.Remove(ctx, collections.Join(c, subject)); err != nil {
			return fmt.Errorf("remove controller index %s -> %s: %w", c, subject, err)
		}
	}
	return nil
}

func (k Keeper) GetDocument(ctx sdk.Context, subject string) (types.DIDDocument, bool) {
	d, err := k.Documents.Get(ctx, subject)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("did doc decode", "subject", subject, "err", err)
		}
		return types.DIDDocument{}, false
	}
	return d, true
}

// IsController returns true iff `addr` is in the document's controller
// set AND the document is active. Used by msg_server gates and by other
// modules (x/device, x/eac) that need "subject must be alive" checks.
func (k Keeper) IsController(ctx sdk.Context, subject, addr string) bool {
	d, ok := k.GetDocument(ctx, subject)
	if !ok || d.Status != types.StatusActive {
		return false
	}
	for _, c := range d.Controllers {
		if c == addr {
			return true
		}
	}
	return false
}

// IsActive is the cheaper variant of IsController for callers that only
// need to know the subject exists in good standing.
func (k Keeper) IsActive(ctx sdk.Context, subject string) bool {
	d, ok := k.GetDocument(ctx, subject)
	return ok && d.Status == types.StatusActive
}

// ---------------------------------------------------------------------------
// Credentials
// ---------------------------------------------------------------------------

// SetCredential writes the credential and (re)indexes by subject, issuer
// and expiry. Callers MUST clear the previous expiry-index entry by hand
// when reissuing a credential whose ExpiresAt changes — only Revoke goes
// through ClearCredentialIndexes.
func (k Keeper) SetCredential(ctx sdk.Context, c types.CredentialStatus) error {
	if err := k.Credentials.Set(ctx, c.Id, c); err != nil {
		return fmt.Errorf("store credential %s: %w", c.Id, err)
	}
	if err := k.CredBySubject.Set(ctx, collections.Join(c.Subject, c.Id)); err != nil {
		return fmt.Errorf("index by subject: %w", err)
	}
	if err := k.CredByIssuer.Set(ctx, collections.Join(c.Issuer, c.Id)); err != nil {
		return fmt.Errorf("index by issuer: %w", err)
	}
	if c.ExpiresAt > 0 {
		if err := k.CredByExpiry.Set(ctx, collections.Join(c.ExpiresAt, c.Id)); err != nil {
			return fmt.Errorf("index by expiry: %w", err)
		}
	}
	return nil
}

// GetCredential returns the row by primary key.
func (k Keeper) GetCredential(ctx sdk.Context, id string) (types.CredentialStatus, bool) {
	c, err := k.Credentials.Get(ctx, id)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("did credential decode", "id", id, "err", err)
		}
		return types.CredentialStatus{}, false
	}
	return c, true
}

// HasValidCredential is the hot-path check used by x/eac, x/rwa and the
// policy DSL: returns true iff the (subject, type) pair has at least one
// row in `valid` state, not yet expired at the chain's BlockTime.
func (k Keeper) HasValidCredential(ctx sdk.Context, subject, credType string) bool {
	now := ctx.BlockTime().Unix()
	found := false
	rng := collections.NewPrefixedPairRange[string, string](subject)
	_ = k.CredBySubject.Walk(ctx, rng, func(key collections.Pair[string, string]) (bool, error) {
		c, ok := k.GetCredential(ctx, key.K2())
		if !ok {
			return false, nil
		}
		if c.Type != credType {
			return false, nil
		}
		if c.Status != types.CredentialValid {
			return false, nil
		}
		if c.ExpiresAt > 0 && c.ExpiresAt < now {
			return false, nil
		}
		found = true
		return true, nil // stop walk early
	})
	return found
}

// ListCredentialsForSubject returns every credential id held by `subject`,
// ignoring revocation status. Caller responsible for filtering. Bounded
// by the keeper-level MaxQueryResults cap.
func (k Keeper) ListCredentialsForSubject(ctx sdk.Context, subject string) []types.CredentialStatus {
	out := make([]types.CredentialStatus, 0, 16)
	rng := collections.NewPrefixedPairRange[string, string](subject)
	_ = k.CredBySubject.Walk(ctx, rng, func(key collections.Pair[string, string]) (bool, error) {
		c, ok := k.GetCredential(ctx, key.K2())
		if ok {
			out = append(out, c)
		}
		if len(out) >= MaxQueryResults {
			return true, nil
		}
		return false, nil
	})
	return out
}

// MarkCredentialRevoked sets status, clears the expiry index entry (which
// no longer applies), and writes back. Called by msg_server.RevokeCredential.
func (k Keeper) MarkCredentialRevoked(ctx sdk.Context, c types.CredentialStatus, reason string) error {
	if c.ExpiresAt > 0 {
		if err := k.CredByExpiry.Remove(ctx, collections.Join(c.ExpiresAt, c.Id)); err != nil {
			return fmt.Errorf("remove expiry index: %w", err)
		}
	}
	c.Status = types.CredentialRevoked
	c.RevokedAt = ctx.BlockTime().Unix()
	c.RevocationReason = reason
	return k.Credentials.Set(ctx, c.Id, c)
}

// ---------------------------------------------------------------------------
// Trust anchors
// ---------------------------------------------------------------------------

func (k Keeper) SetAnchor(ctx sdk.Context, a types.TrustAnchor) error {
	return k.Anchors.Set(ctx, a.Did, a)
}

func (k Keeper) GetAnchor(ctx sdk.Context, did string) (types.TrustAnchor, bool) {
	a, err := k.Anchors.Get(ctx, did)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("did anchor decode", "did", did, "err", err)
		}
		return types.TrustAnchor{}, false
	}
	return a, true
}

// IsActiveAnchor is the read-side check used by msg_server.IssueCredential
// to gate which DIDs can issue. The credential type filter mirrors the
// scope baked into TrustAnchor.CredentialTypes — empty list means "any".
//
// SECURITY: When the anchor is an intermediate, we walk up the parent
// chain and require every ancestor to be Active and well-formed. Without
// this cascade, governance (or a parent root) deactivating a compromised
// root would silently leave its delegated intermediates able to issue
// credentials. The walk is bounded by maxAnchorDepth to keep gas
// predictable; cycles are impossible given the registration-time
// `did != parent_did` check, but the depth bound is a belt-and-braces
// guard in case future migrations introduce one.
func (k Keeper) IsActiveAnchor(ctx sdk.Context, did, credentialType string) bool {
	a, ok := k.GetAnchor(ctx, did)
	if !ok || !a.Active {
		return false
	}
	if !k.anchorChainActive(ctx, a) {
		return false
	}
	if len(a.CredentialTypes) == 0 {
		return true
	}
	for _, t := range a.CredentialTypes {
		if t == credentialType {
			return true
		}
	}
	return false
}

// maxAnchorDepth caps how far we walk an anchor's parent chain before
// giving up. Two tiers (root → intermediate) are the documented model;
// any deeper structure indicates a corrupted store and we conservatively
// refuse to treat the anchor as active rather than risk an unbounded loop.
const maxAnchorDepth = 8

// anchorChainActive walks from `a` up to its root ancestor and returns
// true iff every visited anchor is Active. A missing parent or an
// inactive ancestor causes a `false` return — issuance is gated, not
// silently allowed. Visited DIDs are tracked so a malformed cycle still
// terminates.
func (k Keeper) anchorChainActive(ctx sdk.Context, a types.TrustAnchor) bool {
	visited := map[string]bool{a.Did: true}
	cur := a
	for depth := 0; depth < maxAnchorDepth; depth++ {
		if cur.Tier == types.AnchorTierRoot {
			return cur.Active
		}
		if cur.ParentDid == "" {
			ctx.Logger().Error("did anchor: intermediate without parent", "did", cur.Did)
			return false
		}
		parent, ok := k.GetAnchor(ctx, cur.ParentDid)
		if !ok {
			ctx.Logger().Error("did anchor: missing parent", "did", cur.Did, "parent", cur.ParentDid)
			return false
		}
		if !parent.Active {
			return false
		}
		if visited[parent.Did] {
			ctx.Logger().Error("did anchor: cycle detected", "at", parent.Did)
			return false
		}
		visited[parent.Did] = true
		cur = parent
	}
	ctx.Logger().Error("did anchor: chain too deep", "starting_at", a.Did)
	return false
}

// ---------------------------------------------------------------------------
// Iteration helpers (genesis export, tests)
// ---------------------------------------------------------------------------

// MaxQueryResults bounds keeper-level iteration helpers.
const MaxQueryResults = 1000

func (k Keeper) exportAllDocuments(ctx sdk.Context) []types.DIDDocument {
	var out []types.DIDDocument
	_ = k.Documents.Walk(ctx, nil, func(_ string, v types.DIDDocument) (bool, error) {
		out = append(out, v)
		return false, nil
	})
	return out
}

func (k Keeper) exportAllCredentials(ctx sdk.Context) []types.CredentialStatus {
	var out []types.CredentialStatus
	_ = k.Credentials.Walk(ctx, nil, func(_ string, v types.CredentialStatus) (bool, error) {
		out = append(out, v)
		return false, nil
	})
	return out
}

func (k Keeper) exportAllAnchors(ctx sdk.Context) []types.TrustAnchor {
	var out []types.TrustAnchor
	_ = k.Anchors.Walk(ctx, nil, func(_ string, v types.TrustAnchor) (bool, error) {
		out = append(out, v)
		return false, nil
	})
	return out
}

// ---------------------------------------------------------------------------
// Genesis helpers
// ---------------------------------------------------------------------------

func (k Keeper) InitGenesis(ctx sdk.Context, gs types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return fmt.Errorf("set params: %w", err)
	}
	for _, d := range gs.Documents {
		if err := k.SetDocument(ctx, d); err != nil {
			return fmt.Errorf("import did doc %s: %w", d.Id, err)
		}
	}
	for _, c := range gs.Credentials {
		if err := k.SetCredential(ctx, c); err != nil {
			return fmt.Errorf("import credential %s: %w", c.Id, err)
		}
	}
	for _, a := range gs.Anchors {
		if err := k.SetAnchor(ctx, a); err != nil {
			return fmt.Errorf("import anchor %s: %w", a.Did, err)
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	return &types.GenesisState{
		Params:      k.GetParams(ctx),
		Documents:   k.exportAllDocuments(ctx),
		Credentials: k.exportAllCredentials(ctx),
		Anchors:     k.exportAllAnchors(ctx),
	}
}

// ---------------------------------------------------------------------------
// End-blocker housekeeping
// ---------------------------------------------------------------------------

// SweepExpiredCredentials is called from EndBlock to mark expired
// credentials and trim the expiry index. It walks the [0, now-grace]
// window so each block does bounded work (no full-store scan).
//
// "Mark expired" = set Status to CredentialExpired; we do NOT delete
// because (a) auditors need to see "this credential lapsed at T" and
// (b) downstream `HasValidCredential` already rejects expired rows. Real
// deletion is a future RetentionGC policy concern.
func (k Keeper) SweepExpiredCredentials(ctx sdk.Context) (int, error) {
	now := ctx.BlockTime().Unix()
	if now == 0 {
		return 0, nil // pre-genesis safety
	}
	rng := new(collections.Range[collections.Pair[int64, string]]).
		StartInclusive(collections.PairPrefix[int64, string](0)).
		EndExclusive(collections.PairPrefix[int64, string](now + 1))

	type entry struct {
		expiry int64
		id     string
	}
	var hits []entry
	if err := k.CredByExpiry.Walk(ctx, rng, func(k collections.Pair[int64, string]) (bool, error) {
		hits = append(hits, entry{expiry: k.K1(), id: k.K2()})
		// Bound per-block work so a backlog (long downtime) does not
		// blow the block gas budget. 256 is plenty given typical
		// issuer cadences.
		return len(hits) >= 256, nil
	}); err != nil {
		return 0, fmt.Errorf("walk expiry index: %w", err)
	}

	for _, e := range hits {
		c, ok := k.GetCredential(ctx, e.id)
		if !ok {
			// Index entry without the row is dead weight — clear it.
			_ = k.CredByExpiry.Remove(ctx, collections.Join(e.expiry, e.id))
			continue
		}
		if c.Status == types.CredentialValid {
			c.Status = types.CredentialExpired
			if err := k.Credentials.Set(ctx, c.Id, c); err != nil {
				return 0, fmt.Errorf("mark expired %s: %w", c.Id, err)
			}
		}
		// Whether we marked or not, the index entry is no longer needed.
		if err := k.CredByExpiry.Remove(ctx, collections.Join(e.expiry, e.id)); err != nil {
			return 0, fmt.Errorf("trim expiry index %s: %w", c.Id, err)
		}
	}
	return len(hits), nil
}
