// Package cli wires the x/identity module into Cobra (manual registration;
// pulsar autocli descriptors are not generated for this repo).
package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"

	"energychain/x/identity/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Identity module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterCmd(),
		newUpdateCmd(),
		newRevokeCmd(),
	)
	return cmd
}

// newRegisterCmd: tx identity register <address> <name> <role> [metadata]
func newRegisterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register [address] [name] [role] [metadata]",
		Short: "Register a new identity record",
		Long: `Register an identity. The --from signer is recorded as the
creator. Only the module authority may register identities by default; in
local dev that is the dev0 address.

Example:
  energychaind tx identity register energy1abc... "Solar Plant A" producer \
    '{"capacity_kw":500}' --from dev0 ...`,
		Args: cobra.RangeArgs(3, 4),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			meta := ""
			if len(args) == 4 {
				meta = args[3]
			}
			msg := &types.MsgRegisterIdentity{
				Creator:  clientCtx.GetFromAddress().String(),
				Address:  strings.TrimSpace(args[0]),
				Name:     args[1],
				Role:     args[2],
				Metadata: meta,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newUpdateCmd: tx identity update <address> <name> [metadata]
func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [address] [name] [metadata]",
		Short: "Update an identity's display name and metadata",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			meta := ""
			if len(args) == 3 {
				meta = args[2]
			}
			msg := &types.MsgUpdateIdentity{
				Creator:  clientCtx.GetFromAddress().String(),
				Address:  strings.TrimSpace(args[0]),
				Name:     args[1],
				Metadata: meta,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newRevokeCmd: tx identity revoke <address> [reason]
func newRevokeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke [address] [reason]",
		Short: "Revoke an identity",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			reason := ""
			if len(args) == 2 {
				reason = args[1]
			}
			msg := &types.MsgRevokeIdentity{
				Creator: clientCtx.GetFromAddress().String(),
				Address: strings.TrimSpace(args[0]),
				Reason:  reason,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
