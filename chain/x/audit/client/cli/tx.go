// Package cli wires the x/audit module into Cobra (manual registration;
// pulsar autocli descriptors are not generated for this repo).
package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"

	"energychain/x/audit/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Audit module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(newRecordCmd())
	return cmd
}

// newRecordCmd: tx audit record <event-type> <target> <action> [data]
func newRecordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "record [event-type] [target] [action] [data]",
		Short: "Record an audit event",
		Long: `Append an audit event signed by --from. Use this for compliance
trails (login, settings change, governance proposal, etc).

Example:
  energychaind tx audit record settings energy1abc... update \
    '{"field":"capacity_kw","old":400,"new":500}' \
    --from dev0 ...`,
		Args: cobra.RangeArgs(3, 4),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			data := ""
			if len(args) == 4 {
				data = args[3]
			}
			msg := &types.MsgRecordAudit{
				Creator:   clientCtx.GetFromAddress().String(),
				EventType: strings.TrimSpace(args[0]),
				Target:    strings.TrimSpace(args[1]),
				Action:    strings.TrimSpace(args[2]),
				Data:      data,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
