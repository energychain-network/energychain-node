package evmd

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

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
