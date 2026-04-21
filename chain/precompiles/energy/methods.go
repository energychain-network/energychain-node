package energy

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// energyDataOut mirrors the tuple defined in abi.json for getEnergyData.
// The field order MUST match the components array exactly; go-ethereum's
// abi.Pack uses positional encoding for tuples.
type energyDataOut struct {
	ID          string `abi:"id"`
	Category    string `abi:"category"`
	Submitter   string `abi:"submitter"`
	DataHash    string `abi:"dataHash"`
	Metadata    string `abi:"metadata"`
	BlockHeight int64  `abi:"blockHeight"`
	Timestamp   int64  `abi:"timestamp"`
}

// getEnergyData returns the record stored under the given id, plus a found
// flag. It is the only externally-callable method on the precompile; writes
// are deliberately not exposed (see package doc).
func (p Precompile) getEnergyData(
	ctx sdk.Context,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("getEnergyData expects 1 arg, got %d", len(args))
	}
	id, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("getEnergyData: arg 0 must be string")
	}

	record, found := p.keeper.GetEnergyData(ctx, id)
	out := energyDataOut{
		ID:          record.ID,
		Category:    record.Category,
		Submitter:   record.Submitter,
		DataHash:    record.DataHash,
		Metadata:    record.Metadata,
		BlockHeight: record.BlockHeight,
		Timestamp:   record.Timestamp,
	}
	return method.Outputs.Pack(out, found)
}
