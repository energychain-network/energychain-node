package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// BankKeeper is the slice of x/bank we need: escrow provider bonds into
// the oracle module account and release them back. The interface is
// declared here (rather than imported from x/bank) so unit tests can
// stub the dependency without standing up a full app.
type BankKeeper interface {
	SendCoinsFromAccountToModule(ctx context.Context, from sdk.AccAddress, module string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, module string, to sdk.AccAddress, amt sdk.Coins) error
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
}
