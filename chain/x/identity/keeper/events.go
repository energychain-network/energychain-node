package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/identity/types"
)

// emitEvent appends a typed event with a uniform (action, subject)
// attribute shape so indexers can subscribe per object type.
func emitEvent(ctx sdk.Context, eventType, action, subject string) {
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		eventType,
		sdk.NewAttribute(types.AttrAction, action),
		sdk.NewAttribute(types.AttrSubject, subject),
	))
}
