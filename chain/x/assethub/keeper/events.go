package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/assethub/types"
)

func emitEvent(ctx sdk.Context, eventType, action, subject string, extra ...string) {
	attrs := []sdk.Attribute{
		sdk.NewAttribute(types.AttrAction, action),
		sdk.NewAttribute(types.AttrSubject, subject),
	}
	for i := 0; i+1 < len(extra); i += 2 {
		attrs = append(attrs, sdk.NewAttribute(extra[i], extra[i+1]))
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(eventType, attrs...))
}
