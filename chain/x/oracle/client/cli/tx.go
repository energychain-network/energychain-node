// Package cli provides Cobra commands for the x/oracle module. Registered
// manually because pulsar descriptors used by autocli are not generated.
package cli

import (
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"

	"energychain/x/oracle/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Oracle module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newSubmitDataCmd(),
		newAddOracleCmd(),
		newRemoveOracleCmd(),
	)
	return cmd
}

// newSubmitDataCmd: tx oracle submit <category> <value> [metadata]
func newSubmitDataCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit [category] [value] [metadata]",
		Short: "Submit an oracle data point",
		Long: `Submit a single oracle observation. The signer must be an
authorised oracle. Use --timestamp for a custom unix-second timestamp;
defaults to time.Now().

Example:
  energychaind tx oracle submit price.ecy.usd 1.234 \
    '{"src":"binance"}' --from oracle1 --keyring-backend test \
    --chain-id energychain_9001-1 --gas-prices 10000000000uecy -y`,
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
			ts, _ := cmd.Flags().GetInt64("timestamp")
			if ts == 0 {
				ts = time.Now().Unix()
			}
			msg := &types.MsgSubmitData{
				Submitter: clientCtx.GetFromAddress().String(),
				Category:  strings.TrimSpace(args[0]),
				Value:     strings.TrimSpace(args[1]),
				Metadata:  meta,
				Timestamp: ts,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().Int64("timestamp", 0, "unix-second timestamp; defaults to now")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newAddOracleCmd: tx oracle add-oracle <address> [name]
//
// The signer must be the module authority (gov or admin); for local dev the
// account is dev0 since we set authority = dev0 in genesis. Mainnet should
// route this through governance.
func newAddOracleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-oracle [address] [name]",
		Short: "Authorise a new oracle (signer must be module authority)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			name := ""
			if len(args) == 2 {
				name = args[1]
			}
			cats, _ := cmd.Flags().GetStringSlice("category")
			msg := &types.MsgAddOracle{
				Authority:            clientCtx.GetFromAddress().String(),
				OracleAddress:        strings.TrimSpace(args[0]),
				Name:                 name,
				AuthorizedCategories: cats,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().StringSlice("category", nil, "categories the oracle is authorised to submit (repeatable / comma-separated). Empty = any.")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newRemoveOracleCmd: tx oracle remove-oracle <address>
func newRemoveOracleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove-oracle [address]",
		Short: "Revoke an oracle's submission rights",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			msg := &types.MsgRemoveOracle{
				Authority:     clientCtx.GetFromAddress().String(),
				OracleAddress: strings.TrimSpace(args[0]),
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

var _ = strconv.Itoa
