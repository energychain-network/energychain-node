// Package cli wires the two complex Cobra commands for the
// contract module (create, update-params). The flat single-row
// messages are reached via autocli.
package cli

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"energychain/x/contract/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Contract module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newCreateContractCmd(),
		newUpdateParamsCmd(),
	)
	return cmd
}

type createJSON struct {
	Kind                      string `json:"kind"` // PPA | VPPA | CFD
	Buyer                     string `json:"buyer"`
	Seller                    string `json:"seller"`
	SchemaURI                 string `json:"schema_uri"`
	SchemaHashHex             string `json:"schema_hash_hex"`
	AssetDenom                string `json:"asset_denom"`
	StrikePrice               uint64 `json:"strike_price"`
	NotionalQuantity          uint64 `json:"notional_quantity"`
	PriceOracleTopic          string `json:"price_oracle_topic"`
	QuantityOracleTopic       string `json:"quantity_oracle_topic"`
	MaxOracleStalenessSeconds int64  `json:"max_oracle_staleness_seconds"`
	MarginRequirement         uint64 `json:"margin_requirement"`
	SettlementPeriodSeconds   int64  `json:"settlement_period_seconds"`
	StartTime                 int64  `json:"start_time"`
	EndTime                   int64  `json:"end_time"`
	GracePeriodSeconds        int64  `json:"grace_period_seconds"`
	Memo                      string `json:"memo"`
}

func parseKind(s string) (types.Kind, error) {
	switch strings.ToUpper(s) {
	case "PPA":
		return types.Kind_KIND_PPA, nil
	case "VPPA":
		return types.Kind_KIND_VPPA, nil
	case "CFD":
		return types.Kind_KIND_CFD, nil
	}
	return types.Kind_KIND_UNSPECIFIED, fmt.Errorf("invalid kind %q (PPA|VPPA|CFD)", s)
}

func newCreateContractCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [contract.json]",
		Short: "Create a new bilateral contract (PPA / VPPA / CFD)",
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
			var v createJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			kind, err := parseKind(v.Kind)
			if err != nil {
				return err
			}
			var schemaHash []byte
			if v.SchemaHashHex != "" {
				schemaHash, err = hex.DecodeString(strings.TrimPrefix(v.SchemaHashHex, "0x"))
				if err != nil {
					return fmt.Errorf("schema_hash_hex: %w", err)
				}
			}
			msg := &types.MsgCreateContract{
				Drafter:                   cliCtx.GetFromAddress().String(),
				Kind:                      kind,
				Buyer:                     v.Buyer,
				Seller:                    v.Seller,
				SchemaUri:                 v.SchemaURI,
				SchemaHash:                schemaHash,
				AssetDenom:                v.AssetDenom,
				StrikePrice:               v.StrikePrice,
				NotionalQuantity:          v.NotionalQuantity,
				PriceOracleTopic:          v.PriceOracleTopic,
				QuantityOracleTopic:       v.QuantityOracleTopic,
				MaxOracleStalenessSeconds: v.MaxOracleStalenessSeconds,
				MarginRequirement:         v.MarginRequirement,
				SettlementPeriodSeconds:   v.SettlementPeriodSeconds,
				StartTime:                 v.StartTime,
				EndTime:                   v.EndTime,
				GracePeriodSeconds:        v.GracePeriodSeconds,
				Memo:                      v.Memo,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type paramsJSON struct {
	MaxContracts                 uint32 `json:"max_contracts"`
	MaxContractsPerParty         uint32 `json:"max_contracts_per_party"`
	MinSettlementPeriodSeconds   int64  `json:"min_settlement_period_seconds"`
	MaxSettlementPeriodSeconds   int64  `json:"max_settlement_period_seconds"`
	DefaultGracePeriodSeconds    int64  `json:"default_grace_period_seconds"`
	MaxOracleStalenessSecondsCap int64  `json:"max_oracle_staleness_seconds_cap"`
	MaxCatchupPeriodsPerSettle   uint32 `json:"max_catchup_periods_per_settle"`
	MemoMaxLen                   uint32 `json:"memo_max_len"`
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Update contract module parameters (governance)",
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
					MaxContracts:                 v.MaxContracts,
					MaxContractsPerParty:         v.MaxContractsPerParty,
					MinSettlementPeriodSeconds:   v.MinSettlementPeriodSeconds,
					MaxSettlementPeriodSeconds:   v.MaxSettlementPeriodSeconds,
					DefaultGracePeriodSeconds:    v.DefaultGracePeriodSeconds,
					MaxOracleStalenessSecondsCap: v.MaxOracleStalenessSecondsCap,
					MaxCatchupPeriodsPerSettle:   v.MaxCatchupPeriodsPerSettle,
					MemoMaxLen:                   v.MemoMaxLen,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
