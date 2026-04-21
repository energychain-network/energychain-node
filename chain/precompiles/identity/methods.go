package identity

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	sdk "github.com/cosmos/cosmos-sdk/types"

	identitytypes "energychain/x/identity/types"
)

// identityOut mirrors the tuple defined in abi.json. Field order MUST match
// the components array exactly: go-ethereum encodes tuples positionally.
type identityOut struct {
	Addr         string `abi:"addr"`
	Name         string `abi:"name"`
	Role         string `abi:"role"`
	Status       string `abi:"status"`
	Metadata     string `abi:"metadata"`
	RegisteredAt int64  `abi:"registeredAt"`
	UpdatedAt    int64  `abi:"updatedAt"`
}

func toIdentityOut(id identitytypes.Identity) identityOut {
	return identityOut{
		Addr:         id.Address,
		Name:         id.Name,
		Role:         id.Role,
		Status:       id.Status,
		Metadata:     id.Metadata,
		RegisteredAt: id.RegisteredAt,
		UpdatedAt:    id.UpdatedAt,
	}
}

func parseStringArg(args []interface{}, idx int, name string) (string, error) {
	if len(args) <= idx {
		return "", fmt.Errorf("%s: missing arg %d", name, idx)
	}
	s, ok := args[idx].(string)
	if !ok {
		return "", fmt.Errorf("%s: arg %d must be string", name, idx)
	}
	return s, nil
}

func (p Precompile) getIdentity(
	ctx sdk.Context,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	addr, err := parseStringArg(args, 0, GetIdentityMethod)
	if err != nil {
		return nil, err
	}
	id, found := p.keeper.GetIdentity(ctx, addr)
	return method.Outputs.Pack(toIdentityOut(id), found)
}

// getIdentityByEvmAddress accepts an EVM hex address and looks up the
// identity stored under the matching bech32 string. Useful for contracts
// that already hold the caller's address as `address`.
func (p Precompile) getIdentityByEvmAddress(
	ctx sdk.Context,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%s expects 1 arg, got %d", GetIdentityByEvmAddressMethod, len(args))
	}
	evmAddr, ok := args[0].(common.Address)
	if !ok {
		return nil, fmt.Errorf("%s: arg 0 must be address", GetIdentityByEvmAddressMethod)
	}

	bech32 := sdk.AccAddress(evmAddr.Bytes()).String()
	id, found := p.keeper.GetIdentity(ctx, bech32)
	return method.Outputs.Pack(toIdentityOut(id), found)
}

func (p Precompile) hasRole(
	ctx sdk.Context,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	addr, err := parseStringArg(args, 0, HasRoleMethod)
	if err != nil {
		return nil, err
	}
	role, err := parseStringArg(args, 1, HasRoleMethod)
	if err != nil {
		return nil, err
	}
	return method.Outputs.Pack(p.keeper.HasRole(ctx, addr, role))
}

func (p Precompile) isRegistered(
	ctx sdk.Context,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	addr, err := parseStringArg(args, 0, IsRegisteredMethod)
	if err != nil {
		return nil, err
	}
	return method.Outputs.Pack(p.keeper.IsRegistered(ctx, addr))
}
