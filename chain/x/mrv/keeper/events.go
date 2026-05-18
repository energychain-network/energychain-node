package keeper

import (
	"context"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// emit is a tiny wrapper around the SDK event manager that
// the mrv keeper uses for every observable transition. Keep
// each event's type stable so off-chain indexers can rely on
// it; new attributes are additive, never removed.
func (k Keeper) emit(ctx context.Context, typ string, attrs ...sdk.Attribute) {
	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(sdk.NewEvent(typ, attrs...))
}

func u64s(v uint64) string { return strconv.FormatUint(v, 10) }
func i64s(v int64) string  { return strconv.FormatInt(v, 10) }
