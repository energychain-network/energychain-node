// Package cli wires the governance-only update-params command for
// streampay. All other Msg shapes are flat enough that autocli
// covers them.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"energychain/x/streampay/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Streampay module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(newUpdateParamsCmd())
	return cmd
}

type paramsJSON struct {
	MaxStreams          uint32 `json:"max_streams"`
	MaxStreamsPerSender uint32 `json:"max_streams_per_sender"`
	MinRatePerSecond    uint64 `json:"min_rate_per_second"`
	MaxRatePerSecond    uint64 `json:"max_rate_per_second"`
	MaxDeposit          uint64 `json:"max_deposit"`
	MaxHorizonSeconds   int64  `json:"max_horizon_seconds"`
	DefaultDenom        string `json:"default_denom"`
	MemoMaxLen          uint32 `json:"memo_max_len"`
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Update streampay module parameters (governance)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			bz, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read %s: %w", args[0], err)
			}
			var v paramsJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			msg := &types.MsgUpdateParams{
				Authority: cliCtx.GetFromAddress().String(),
				Params: types.Params{
					MaxStreams:          v.MaxStreams,
					MaxStreamsPerSender: v.MaxStreamsPerSender,
					MinRatePerSecond:    v.MinRatePerSecond,
					MaxRatePerSecond:    v.MaxRatePerSecond,
					MaxDeposit:          v.MaxDeposit,
					MaxHorizonSeconds:   v.MaxHorizonSeconds,
					DefaultDenom:        v.DefaultDenom,
					MemoMaxLen:          v.MemoMaxLen,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
