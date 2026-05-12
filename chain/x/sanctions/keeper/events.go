package keeper

import sdk "github.com/cosmos/cosmos-sdk/types"

func emit(ctx sdk.Context, kind string, kvs ...string) {
	if len(kvs)%2 != 0 {
		ctx.Logger().Error("sanctions: emit got odd kvs", "kind", kind)
		return
	}
	attrs := make([]sdk.Attribute, 0, len(kvs)/2)
	for i := 0; i < len(kvs); i += 2 {
		attrs = append(attrs, sdk.NewAttribute(kvs[i], kvs[i+1]))
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(kind, attrs...))
}
