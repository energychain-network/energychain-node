package testutil

import (
	"crypto/sha256"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DeriveAddr returns a deterministic, valid 20-byte bech32 account
// address derived from seed. Using a hash keeps addresses stable
// across runs while guaranteeing they round-trip through
// sdk.AccAddressFromBech32 regardless of the chain's configured
// bech32 prefix.
func DeriveAddr(seed string) string {
	h := sha256.Sum256([]byte("energychain/testutil/" + seed))
	return sdk.AccAddress(h[:20]).String()
}

// Addrs returns n deterministic distinct addresses.
func Addrs(n int) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = DeriveAddr(string(rune('a'+i)) + "-acct")
	}
	return out
}
