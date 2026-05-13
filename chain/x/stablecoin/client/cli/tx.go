// Package cli provides Cobra commands for the x/stablecoin module.
// Messages with structured payloads (RegisterDenom, RegisterIssuer,
// UpdateIssuer, UpdateParams) live here because autocli cannot
// synthesise the multi-flag list shapes they need.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"energychain/x/stablecoin/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Stablecoin module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterDenomCmd(),
		newUpdateDenomCmd(),
		newRegisterIssuerCmd(),
		newUpdateIssuerCmd(),
		newUpdateParamsCmd(),
	)
	return cmd
}

// ---- Denom -----------------------------------------------------------------

type denomJSON struct {
	ID           string `json:"id"`
	Symbol       string `json:"symbol"`
	Name         string `json:"name"`
	Decimals     uint32 `json:"decimals"`
	Jurisdiction string `json:"jurisdiction"`
	PolicyID     string `json:"policy_id"`
}

func readDenomJSON(path string) (denomJSON, error) {
	bz, err := os.ReadFile(path)
	if err != nil {
		return denomJSON{}, fmt.Errorf("read %s: %w", path, err)
	}
	var d denomJSON
	if err := json.Unmarshal(bz, &d); err != nil {
		return denomJSON{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return d, nil
}

func newRegisterDenomCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-denom [denom.json]",
		Short: "Register a new stablecoin denom from a JSON file (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			d, err := readDenomJSON(args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgRegisterDenom{
				Authority:    cliCtx.GetFromAddress().String(),
				Id:           d.ID,
				Symbol:       d.Symbol,
				Name:         d.Name,
				Decimals:     d.Decimals,
				Jurisdiction: d.Jurisdiction,
				PolicyId:     d.PolicyID,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newUpdateDenomCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-denom [denom.json]",
		Short: "Update an existing stablecoin denom from a JSON file (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			d, err := readDenomJSON(args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgUpdateDenom{
				Authority:    cliCtx.GetFromAddress().String(),
				Id:           d.ID,
				Symbol:       d.Symbol,
				Name:         d.Name,
				Jurisdiction: d.Jurisdiction,
				PolicyId:     d.PolicyID,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// ---- Issuer ----------------------------------------------------------------

type issuerJSON struct {
	ID              string   `json:"id"`
	DID             string   `json:"did"`
	DisplayName     string   `json:"display_name"`
	MintAuthorities []string `json:"mint_authorities"`
	Admin           string   `json:"admin"`
}

func readIssuerJSON(path string) (issuerJSON, error) {
	bz, err := os.ReadFile(path)
	if err != nil {
		return issuerJSON{}, fmt.Errorf("read %s: %w", path, err)
	}
	var is issuerJSON
	if err := json.Unmarshal(bz, &is); err != nil {
		return issuerJSON{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return is, nil
}

func newRegisterIssuerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-issuer [issuer.json]",
		Short: "Register a new stablecoin issuer from a JSON file (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			is, err := readIssuerJSON(args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgRegisterIssuer{
				Authority:       cliCtx.GetFromAddress().String(),
				Id:              is.ID,
				Did:             is.DID,
				DisplayName:     is.DisplayName,
				MintAuthorities: is.MintAuthorities,
				Admin:           is.Admin,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newUpdateIssuerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-issuer [issuer.json]",
		Short: "Update an issuer's mint authorities / admin / display name (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			is, err := readIssuerJSON(args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgUpdateIssuer{
				Authority:       cliCtx.GetFromAddress().String(),
				Id:              is.ID,
				DisplayName:     is.DisplayName,
				MintAuthorities: is.MintAuthorities,
				Admin:           is.Admin,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// ---- Params ---------------------------------------------------------------

func readParamsJSON(path string) (types.Params, error) {
	bz, err := os.ReadFile(path)
	if err != nil {
		return types.Params{}, fmt.Errorf("read %s: %w", path, err)
	}
	var p types.Params
	if err := json.Unmarshal(bz, &p); err != nil {
		return types.Params{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return p, nil
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Submit a stablecoin params update (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			p, err := readParamsJSON(args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgUpdateParams{
				Authority: cliCtx.GetFromAddress().String(),
				Params:    p,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
