// Package cli wires the x/energy module into Cobra so operators can sign
// transactions from the command line. We register these manually because the
// repo does not generate the pulsar-style proto descriptors that the
// autocli framework needs to enhance commands automatically.
package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"

	"energychain/x/energy/types"
)

// GetTxCmd returns the energy module root tx command. Add it to the root
// `tx` command so users can sign and broadcast x/energy messages directly.
func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Energy module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newSubmitCmd(),
		newBatchSubmitCmd(),
	)
	return cmd
}

// newSubmitCmd builds: tx energy submit <category> <data_hash> [metadata]
func newSubmitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit [category] [data-hash] [metadata]",
		Short: "Submit a single energy data record",
		Long: `Submit a single energy data record signed by --from.

Example:
  energychaind tx energy submit solar 0xaabb \
    '{"kwh":120,"unit":"hour"}' \
    --from dev0 --keyring-backend test \
    --chain-id energychain_9001-1 \
    --gas 250000 --gas-prices 10000000000uecy -y`,
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			meta := ""
			if len(args) == 3 {
				meta = args[2]
			}
			msg := &types.MsgSubmitEnergyData{
				Submitter: clientCtx.GetFromAddress().String(),
				Category:  args[0],
				DataHash:  normaliseHex(args[1]),
				Metadata:  meta,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newBatchSubmitCmd builds: tx energy batch-submit <category> <merkle_root> --items=<json>
//
// items is a JSON array like [{"data_hash":"0x..","metadata":"{...}"}, ...]
// We accept either bare hex hashes or {data_hash, metadata} objects.
func newBatchSubmitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch-submit [category] [merkle-root]",
		Short: "Submit a batch of energy data records",
		Long: `Submit a Merkle batch of energy records.

The batch's items are passed via --items as a JSON array, e.g.

  --items='[
    {"data_hash":"0xaa","metadata":"{\"kwh\":1}"},
    {"data_hash":"0xbb","metadata":"{\"kwh\":2}"}
  ]'`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			itemsRaw, _ := cmd.Flags().GetString("items")
			items, err := parseBatchItems(itemsRaw)
			if err != nil {
				return fmt.Errorf("--items: %w", err)
			}
			msg := &types.MsgBatchSubmit{
				Submitter:  clientCtx.GetFromAddress().String(),
				Category:   args[0],
				Items:      items,
				MerkleRoot: normaliseHex(args[1]),
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String("items", "[]", "JSON array of {data_hash, metadata} batch items")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// parseBatchItems decodes the --items flag into the proto BatchItem slice.
//
// Accepted shapes:
//   ["0xaa","0xbb"]                                          (hashes only)
//   [{"data_hash":"0xaa"},{"data_hash":"0xbb","metadata":"..."}]
func parseBatchItems(raw string) ([]types.BatchItem, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil, nil
	}
	// Try rich form first.
	var rich []struct {
		DataHash string `json:"data_hash"`
		Metadata string `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(raw), &rich); err == nil && len(rich) > 0 && rich[0].DataHash != "" {
		out := make([]types.BatchItem, 0, len(rich))
		for _, r := range rich {
			out = append(out, types.BatchItem{
				DataHash: normaliseHex(r.DataHash),
				Metadata: r.Metadata,
			})
		}
		return out, nil
	}
	// Fall back to plain hash list.
	var hashes []string
	if err := json.Unmarshal([]byte(raw), &hashes); err != nil {
		return nil, fmt.Errorf("invalid items json: %w", err)
	}
	out := make([]types.BatchItem, 0, len(hashes))
	for _, h := range hashes {
		out = append(out, types.BatchItem{DataHash: normaliseHex(h)})
	}
	return out, nil
}

// normaliseHex makes sure on-chain payloads always carry a `0x` prefix when
// the user pastes raw hex, so explorers / decoders can detect the encoding.
func normaliseHex(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return s
	}
	if isHex(s) {
		return "0x" + s
	}
	return s
}

func isHex(s string) bool {
	if len(s)%2 != 0 || len(s) == 0 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

