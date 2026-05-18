package evmd

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"

	cfe247types "energychain/x/cfe247/types"
	eackeeper "energychain/x/eac/keeper"
	eactypes "energychain/x/eac/types"
	oraclekeeper "energychain/x/oracle/keeper"
	rwakeeper "energychain/x/rwa/keeper"
	stablecoinkeeper "energychain/x/stablecoin/keeper"
)

// stablecoinOracleAdapter bridges the rich x/oracle.AggregatedValue
// type into x/stablecoin's narrow OracleKeeper expected interface,
// which only needs (value, timestamp, ok). Keeping the adapter inside
// chain/app.go area (as opposed to inside the stablecoin module)
// preserves the modules' independence — x/stablecoin does not import
// x/oracle.
type stablecoinOracleAdapter struct {
	k oraclekeeper.Keeper
}

// GetAggregatedReserve returns the latest aggregated value for a
// reserve attestation topic. AggregatedValue.Value is int64 in
// x/oracle to allow for signed metrics; the stablecoin keeper rejects
// negative values upstream as "missing".
func (a stablecoinOracleAdapter) GetAggregatedReserve(ctx sdk.Context, topicID string) (value int64, timestamp int64, ok bool) {
	v, found := a.k.GetAggregated(ctx, topicID)
	if !found {
		return 0, 0, false
	}
	return v.Value, v.ComputedTime, true
}

// eacOracleAdapter bridges x/oracle.AggregatedValue into x/eac's
// narrow OracleKeeper expected interface for cross-registry bridge
// attestations. The shape matches stablecoinOracleAdapter so the same
// underlying x/oracle topics can be reused by both modules.
type eacOracleAdapter struct {
	k oraclekeeper.Keeper
}

func (a eacOracleAdapter) GetAggregatedReserve(ctx sdk.Context, topicID string) (value int64, timestamp int64, ok bool) {
	v, found := a.k.GetAggregated(ctx, topicID)
	if !found {
		return 0, 0, false
	}
	return v.Value, v.ComputedTime, true
}

// cfe247EACAdapter is the read-only join cfe247 uses to look up an
// x/eac retirement together with the certificate metadata it needs
// for hour / zone / technology checks. Implementations MUST NOT
// mutate state; the cfe247 keeper relies on this guarantee for its
// own deterministic accounting.
type cfe247EACAdapter struct {
	k eackeeper.Keeper
}

func (a cfe247EACAdapter) LookupRetirement(ctx sdk.Context, retirementID uint64) (cfe247types.EACRetirementView, bool) {
	r, err := a.k.Retirements.Get(ctx, retirementID)
	if err != nil {
		return cfe247types.EACRetirementView{}, false
	}
	c, err := a.k.Certificates.Get(ctx, r.CertificateId)
	if err != nil {
		return cfe247types.EACRetirementView{}, false
	}
	return cfe247types.EACRetirementView{
		RetirementID:  r.Id,
		CertificateID: c.Id,
		Retirer:       r.Retirer,
		Beneficiary:   r.Beneficiary,
		Amount:        r.Amount,
		CertHourStart: c.HourStart,
		CertHourEnd:   c.HourEnd,
		GridZone:      c.GridZone,
		Technology:    int32(c.Technology),
		IsStorage:     c.Technology == eactypes.Technology_TECHNOLOGY_STORAGE,
	}, true
}

// carbonOracleAdapter mirrors eacOracleAdapter for x/carbon's
// cross-registry bridge attestations (Verra, Toucan, Klima,
// Article 6.4, etc.). The shape matches both stablecoin and EAC so
// the same underlying x/oracle topics can be reused.
type carbonOracleAdapter struct {
	k oraclekeeper.Keeper
}

func (a carbonOracleAdapter) GetAggregatedReserve(ctx sdk.Context, topicID string) (value int64, timestamp int64, ok bool) {
	v, found := a.k.GetAggregated(ctx, topicID)
	if !found {
		return 0, 0, false
	}
	return v.Value, v.ComputedTime, true
}

// rwaStablecoinAdapter is the thin x/rwa → x/stablecoin bridge for
// dividend payouts and redemption settlements. It exposes only the
// HasDenom + Move surface that x/rwa.types.StablecoinKeeper requires.
//
// Move() invokes x/stablecoin's MoveBalance which intentionally
// bypasses the per-account compliance pipeline (sanctions / freeze /
// blacklist) at the stablecoin layer; the x/rwa keeper enforces all
// such gates at the RWA-token level before invoking Move(), and the
// stablecoin module-controlled distribution pool account is not a
// real holder so applying its own freeze checks would be
// inappropriate.
type rwaStablecoinAdapter struct {
	k stablecoinkeeper.Keeper
}

func (a rwaStablecoinAdapter) HasDenom(ctx sdk.Context, denomID string) bool {
	return a.k.HasDenom(ctx, denomID)
}

func (a rwaStablecoinAdapter) Move(ctx sdk.Context, denomID, from, to string, amount uint64) error {
	return a.k.MoveBalance(ctx, denomID, from, to, amount)
}

// escrowStablecoinAdapter is the same shape as rwaStablecoinAdapter
// but bound to x/escrow's StablecoinKeeper interface. Splitting the
// adapter — instead of sharing rwaStablecoinAdapter — makes it
// easier to evolve each module's settlement contract independently
// (e.g. adding a per-escrow allowance flow without entangling
// dividend payouts).
type escrowStablecoinAdapter struct {
	k stablecoinkeeper.Keeper
}

func (a escrowStablecoinAdapter) HasDenom(ctx sdk.Context, denomID string) bool {
	return a.k.HasDenom(ctx, denomID)
}

func (a escrowStablecoinAdapter) Move(ctx sdk.Context, denomID, from, to string, amount uint64) error {
	return a.k.MoveBalance(ctx, denomID, from, to, amount)
}

func (a escrowStablecoinAdapter) IsAccountBlocked(ctx sdk.Context, denomID, account string) bool {
	return a.k.IsAccountBlocked(ctx, denomID, account)
}

func (a escrowStablecoinAdapter) IsDenomPaused(ctx sdk.Context, denomID string) bool {
	return a.k.IsDenomPaused(ctx, denomID)
}

// escrowRWAAdapter exposes the narrow RWAKeeper surface x/escrow
// expects (HasToken / EscrowLock / EscrowRelease). The lock/release
// pair embeds full holder-side compliance on the release leg
// (KYC / per-holder cap / sanctions) and transferable check on the
// lock leg (frozen / lockup); see x/rwa/keeper/escrow.go.
type escrowRWAAdapter struct {
	k rwakeeper.Keeper
}

func (a escrowRWAAdapter) HasToken(ctx sdk.Context, tokenID uint64) bool {
	return a.k.HasToken(ctx, tokenID)
}

func (a escrowRWAAdapter) EscrowLock(ctx sdk.Context, tokenID uint64, depositor string, amount uint64) error {
	return a.k.EscrowLock(ctx, tokenID, depositor, amount)
}

func (a escrowRWAAdapter) EscrowRelease(ctx sdk.Context, tokenID uint64, recipient string, amount uint64) error {
	return a.k.EscrowRelease(ctx, tokenID, recipient, amount)
}

// escrowOracleAdapter shares its shape with stablecoin / eac /
// carbon adapters: every consumer of x/oracle aggregated values
// reads (value, timestamp, ok) from the same surface, so a single
// underlying topic can be reused across modules.
type escrowOracleAdapter struct {
	k oraclekeeper.Keeper
}

func (a escrowOracleAdapter) GetAggregatedReserve(ctx sdk.Context, topicID string) (value int64, timestamp int64, ok bool) {
	v, found := a.k.GetAggregated(ctx, topicID)
	if !found {
		return 0, 0, false
	}
	return v.Value, v.ComputedTime, true
}

// contractOracleAdapter shares its shape with stablecoin / eac /
// carbon / escrow adapters: every consumer of x/oracle aggregated
// values reads (value, timestamp, ok) from the same surface. The
// x/contract settlement engine validates the timestamp against
// each contract's max_oracle_staleness_seconds so we do NOT
// pre-filter staleness here — the per-contract policy is the
// source of truth.
type contractOracleAdapter struct {
	k oraclekeeper.Keeper
}

func (a contractOracleAdapter) GetAggregatedReserve(ctx sdk.Context, topicID string) (value int64, timestamp int64, ok bool) {
	v, found := a.k.GetAggregated(ctx, topicID)
	if !found {
		return 0, 0, false
	}
	return v.Value, v.ComputedTime, true
}

// schedulerMsgRouterAdapter wraps BaseApp.MsgServiceRouter so the
// x/scheduler keeper can route scheduled payloads at tick time
// without taking a direct dependency on the SDK service router.
//
// RouteMsg returns the handler's first error. Successful handler
// returns are surfaced as nil regardless of any sdk.Result fields —
// the per-tick fee accounting and audit live in x/scheduler, not
// here.
type schedulerMsgRouterAdapter struct {
	msr *baseapp.MsgServiceRouter
}

func (a schedulerMsgRouterAdapter) RouteMsg(ctx sdk.Context, msg sdk.Msg) error {
	if a.msr == nil {
		return fmt.Errorf("msg service router not wired")
	}
	handler := a.msr.Handler(msg)
	if handler == nil {
		return fmt.Errorf("no handler registered for %T", msg)
	}
	_, err := handler(ctx, msg)
	return err
}
