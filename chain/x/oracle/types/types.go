package types

const (
	ModuleName = "oracle"
	StoreKey   = ModuleName
	RouterKey  = ModuleName
)

// Hard upper/lower bounds enforced by Params.Validate().
const (
	OracleMaxMetadataLowerBound = 128
	OracleMaxMetadataUpperBound = 65_536
	OracleDataMaxAgeUpperBound  = 31_536_000 // 1 year
	DefaultOracleMaxMetadata    = 4_096
)

// IsAuthorizedFor checks whether the oracle is active and authorized for the
// given category. An empty AuthorizedCategories list means the oracle can
// submit any category.
//
// OracleData / OracleInfo / Params are now generated from
// proto/energychain/oracle/v1/types.proto.
func (o OracleInfo) IsAuthorizedFor(category string) bool {
	if !o.Active {
		return false
	}
	if len(o.AuthorizedCategories) == 0 {
		return true
	}
	for _, c := range o.AuthorizedCategories {
		if c == category {
			return true
		}
	}
	return false
}
