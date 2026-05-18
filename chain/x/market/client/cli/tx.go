// Package cli wires the Cobra commands that are too rich for
// autocli (CreatePair, PlaceLimitOrder, PauseUnpausePair,
// UpdatePairRisk, UpdateParams). Cancel and ClearBatch are
// reached via autocli.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"energychain/x/market/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Market module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newCreatePairCmd(),
		newPlaceLimitOrderCmd(),
		newPauseUnpauseCmd(),
		newUpdatePairRiskCmd(),
		newUpdateParamsCmd(),
	)
	return cmd
}

type pairJSON struct {
	BaseDenom            string `json:"base_denom"`
	QuoteDenom           string `json:"quote_denom"`
	Mode                 string `json:"mode"` // CONTINUOUS | FBA
	BatchIntervalSeconds int64  `json:"batch_interval_seconds"`
	PriceBandLo          uint64 `json:"price_band_lo"`
	PriceBandHi          uint64 `json:"price_band_hi"`
	MaxPositionPerUser   uint64 `json:"max_position_per_user"`
	Memo                 string `json:"memo"`
}

func parseMode(s string) (types.MatchMode, error) {
	switch strings.ToUpper(s) {
	case "CONTINUOUS":
		return types.MatchMode_MATCH_MODE_CONTINUOUS, nil
	case "FBA":
		return types.MatchMode_MATCH_MODE_FBA, nil
	}
	return types.MatchMode_MATCH_MODE_UNSPECIFIED, fmt.Errorf("invalid mode %q (CONTINUOUS|FBA)", s)
}

func newCreatePairCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-pair [pair.json]",
		Short: "Create a tradable pair (governance)",
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
			var v pairJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			mode, err := parseMode(v.Mode)
			if err != nil {
				return err
			}
			msg := &types.MsgCreatePair{
				Authority:            cliCtx.GetFromAddress().String(),
				BaseDenom:            v.BaseDenom,
				QuoteDenom:           v.QuoteDenom,
				Mode:                 mode,
				BatchIntervalSeconds: v.BatchIntervalSeconds,
				PriceBandLo:          v.PriceBandLo,
				PriceBandHi:          v.PriceBandHi,
				MaxPositionPerUser:   v.MaxPositionPerUser,
				Memo:                 v.Memo,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func parseSide(s string) (types.Side, error) {
	switch strings.ToUpper(s) {
	case "BUY":
		return types.Side_SIDE_BUY, nil
	case "SELL":
		return types.Side_SIDE_SELL, nil
	}
	return types.Side_SIDE_UNSPECIFIED, fmt.Errorf("invalid side %q (BUY|SELL)", s)
}

func newPlaceLimitOrderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "place [pair-id] [side] [price] [quantity] [memo]",
		Short: "Place a limit order",
		Args:  cobra.RangeArgs(4, 5),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			pid, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("pair_id: %w", err)
			}
			side, err := parseSide(args[1])
			if err != nil {
				return err
			}
			price, err := strconv.ParseUint(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("price: %w", err)
			}
			qty, err := strconv.ParseUint(args[3], 10, 64)
			if err != nil {
				return fmt.Errorf("quantity: %w", err)
			}
			memo := ""
			if len(args) == 5 {
				memo = args[4]
			}
			msg := &types.MsgPlaceLimitOrder{
				Owner: cliCtx.GetFromAddress().String(),
				PairId: pid, Side: side, Price: price, Quantity: qty, Memo: memo,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newPauseUnpauseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pause [pair-id] [pause|unpause] [reason]",
		Short: "Pause or unpause a pair (governance)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			pid, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("pair_id: %w", err)
			}
			var pause bool
			switch strings.ToLower(args[1]) {
			case "pause":
				pause = true
			case "unpause":
				pause = false
			default:
				return fmt.Errorf("expected pause|unpause, got %q", args[1])
			}
			msg := &types.MsgPauseUnpausePair{
				Authority: cliCtx.GetFromAddress().String(),
				PairId:    pid,
				Pause:     pause,
				Reason:    args[2],
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newUpdatePairRiskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-risk [pair-id] [band-lo] [band-hi] [max-position]",
		Short: "Update pair risk gates (governance)",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			pid, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("pair_id: %w", err)
			}
			lo, err := strconv.ParseUint(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("band_lo: %w", err)
			}
			hi, err := strconv.ParseUint(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("band_hi: %w", err)
			}
			cap, err := strconv.ParseUint(args[3], 10, 64)
			if err != nil {
				return fmt.Errorf("max_position: %w", err)
			}
			msg := &types.MsgUpdatePairRisk{
				Authority:          cliCtx.GetFromAddress().String(),
				PairId:             pid,
				PriceBandLo:        lo,
				PriceBandHi:        hi,
				MaxPositionPerUser: cap,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type paramsJSON struct {
	MaxPairs                    uint32 `json:"max_pairs"`
	MaxOpenOrdersPerPair        uint32 `json:"max_open_orders_per_pair"`
	MaxOpenOrdersPerUserPerPair uint32 `json:"max_open_orders_per_user_per_pair"`
	MaxFillsPerMatch            uint32 `json:"max_fills_per_match"`
	MaxMatchesPerClear          uint32 `json:"max_matches_per_clear"`
	MemoMaxLen                  uint32 `json:"memo_max_len"`
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Update market module parameters (governance)",
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
					MaxPairs:                    v.MaxPairs,
					MaxOpenOrdersPerPair:        v.MaxOpenOrdersPerPair,
					MaxOpenOrdersPerUserPerPair: v.MaxOpenOrdersPerUserPerPair,
					MaxFillsPerMatch:            v.MaxFillsPerMatch,
					MaxMatchesPerClear:          v.MaxMatchesPerClear,
					MemoMaxLen:                  v.MemoMaxLen,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
