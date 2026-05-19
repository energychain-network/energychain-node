package eac

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// evmAddrToBech32 mirrors the conversion used by every cosmos-evm
// precompile (sdk.AccAddress(evmAddr.Bytes()).String()).
func evmAddrToBech32(addr [20]byte) string {
	return sdk.AccAddress(addr[:]).String()
}

// BalanceOf returns keeper.GetBalance(certID, holder). Keeper errors
// (unknown cert id, decoder failure) surface here so callers can
// distinguish "0 balance" from "lookup failed".
func (p Precompile) BalanceOf(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	certID, holder, err := parseBalanceOfArgs(args)
	if err != nil {
		return nil, err
	}
	bal, err := p.keeper.GetBalance(ctx, certID, evmAddrToBech32(holder))
	if err != nil {
		// Keeper returns an error on storage decode failure; surface
		// as a revert so the caller never sees a silent zero.
		return nil, err
	}
	return method.Outputs.Pack(new(big.Int).SetUint64(bal))
}

// CertificateIssuedUnits returns (issuedUnits, exists). exists=false
// when the certificate has never been registered.
func (p Precompile) CertificateIssuedUnits(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	certID, err := parseCertIDArg(args)
	if err != nil {
		return nil, err
	}
	units, ok := p.keeper.GetCertificateIssuedUnits(ctx, certID)
	return method.Outputs.Pack(new(big.Int).SetUint64(units), ok)
}
