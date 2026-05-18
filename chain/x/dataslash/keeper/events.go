package keeper

import (
	"context"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (k Keeper) emit(ctx context.Context, typ string, attrs ...sdk.Attribute) {
	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(sdk.NewEvent(typ, attrs...))
}

func u64s(v uint64) string { return strconv.FormatUint(v, 10) }
func u32s(v uint32) string { return strconv.FormatUint(uint64(v), 10) }
func i64s(v int64) string  { return strconv.FormatInt(v, 10) }
