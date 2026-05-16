package keeper

import (
	"context"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (Keeper) emit(ctx context.Context, ev string, kvs ...string) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	attrs := make([]sdk.Attribute, 0, len(kvs)/2)
	for i := 0; i+1 < len(kvs); i += 2 {
		attrs = append(attrs, sdk.NewAttribute(kvs[i], kvs[i+1]))
	}
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent("scheduler."+ev, attrs...))
}

func u64s(v uint64) string { return strconv.FormatUint(v, 10) }
func i64s(v int64) string  { return strconv.FormatInt(v, 10) }
