package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// BankKeeper custodies provider bonds in the module account.
type BankKeeper interface {
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	BurnCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
}

// AuditKeeper is the optional cross-module audit log (satisfied by
// x/identity). It may be nil, in which case audit recording is skipped.
type AuditKeeper interface {
	RecordAction(ctx sdk.Context, module, action, actor, subject, detail string)
}
