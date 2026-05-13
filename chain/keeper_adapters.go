package evmd

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	cfe247types "energychain/x/cfe247/types"
	eackeeper "energychain/x/eac/keeper"
	eactypes "energychain/x/eac/types"
	oraclekeeper "energychain/x/oracle/keeper"
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
