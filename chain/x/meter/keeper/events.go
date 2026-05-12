package keeper

import sdk "github.com/cosmos/cosmos-sdk/types"

// emit is a tiny helper that builds a string-attribute event without
// dragging the eventattr package import everywhere. Audit / dataslash
// modules consume the emitted attributes verbatim.
func emit(ctx sdk.Context, kind string, kvs ...string) {
	if len(kvs)%2 != 0 {
		ctx.Logger().Error("meter: emit got odd kvs", "kind", kind)
		return
	}
	attrs := make([]sdk.Attribute, 0, len(kvs)/2)
	for i := 0; i < len(kvs); i += 2 {
		attrs = append(attrs, sdk.NewAttribute(kvs[i], kvs[i+1]))
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(kind, attrs...))
}
