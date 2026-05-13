package keeper

import sdk "github.com/cosmos/cosmos-sdk/types"

func emit(ctx sdk.Context, ev string, attrs ...[2]string) {
	out := make([]sdk.Attribute, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, sdk.NewAttribute(a[0], a[1]))
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(ev, out...))
}
