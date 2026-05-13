package keeper

import sdk "github.com/cosmos/cosmos-sdk/types"

// emit is the small typed-event helper used by every msg server entry.
// kvs is a flat (key, value, key, value, ...) slice; an odd count is
// a programming bug — we log and drop the event rather than panic so
// a typo can't halt the chain.
func emit(ctx sdk.Context, kind string, kvs ...string) {
	if len(kvs)%2 != 0 {
		ctx.Logger().Error("stablecoin: emit got odd kvs", "kind", kind)
		return
	}
	attrs := make([]sdk.Attribute, 0, len(kvs)/2)
	for i := 0; i < len(kvs); i += 2 {
		attrs = append(attrs, sdk.NewAttribute(kvs[i], kvs[i+1]))
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(kind, attrs...))
}
