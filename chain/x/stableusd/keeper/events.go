package keeper

import sdk "github.com/cosmos/cosmos-sdk/types"

func emitEvent(ctx sdk.Context, eventType string, attrs ...string) {
	a := make([]sdk.Attribute, 0, len(attrs)/2)
	for i := 0; i+1 < len(attrs); i += 2 {
		a = append(a, sdk.NewAttribute(attrs[i], attrs[i+1]))
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(eventType, a...))
}
