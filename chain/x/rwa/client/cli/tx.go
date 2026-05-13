// Package cli provides Cobra commands for the x/rwa module.
// Messages with multi-flag JSON payloads (RegisterIssuer / UpdateIssuer
// / CreateToken / UpdateToken / SetAccountFlags / CreateDistribution
// / UpdateParams) live here because autocli cannot synthesise the
// nested shapes they need.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"energychain/x/rwa/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "RWA module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterIssuerCmd(),
		newUpdateIssuerCmd(),
		newCreateTokenCmd(),
		newUpdateTokenCmd(),
		newSetAccountFlagsCmd(),
		newCreateDistributionCmd(),
		newUpdateParamsCmd(),
	)
	return cmd
}

func readJSON(path string, out any) error {
	bz, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(bz, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// ---- Issuer ---------------------------------------------------------------

type issuerJSON struct {
	ID              string `json:"id"`
	DID             string `json:"did"`
	DisplayName     string `json:"display_name"`
	IssuerAuthority string `json:"issuer_authority"`
	IssuerAdmin     string `json:"issuer_admin"`
}

func newRegisterIssuerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-issuer [issuer.json]",
		Short: "Register a new RWA issuer from a JSON file (governance)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var v issuerJSON
			if err := readJSON(args[0], &v); err != nil {
				return err
			}
			msg := &types.MsgRegisterIssuer{
				Authority:       cliCtx.GetFromAddress().String(),
				Id:              v.ID,
				Did:             v.DID,
				DisplayName:     v.DisplayName,
				IssuerAuthority: v.IssuerAuthority,
				IssuerAdmin:     v.IssuerAdmin,
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
		Short: "Update an existing RWA issuer (governance)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var v issuerJSON
			if err := readJSON(args[0], &v); err != nil {
				return err
			}
			msg := &types.MsgUpdateIssuer{
				Authority:       cliCtx.GetFromAddress().String(),
				Id:              v.ID,
				DisplayName:     v.DisplayName,
				IssuerAuthority: v.IssuerAuthority,
				IssuerAdmin:     v.IssuerAdmin,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// ---- Token ----------------------------------------------------------------

type createTokenJSON struct {
	IssuerID               string `json:"issuer_id"`
	Symbol                 string `json:"symbol"`
	DisplayName            string `json:"display_name"`
	Decimals               uint32 `json:"decimals"`
	AssetClass             int32  `json:"asset_class"`
	Jurisdiction           string `json:"jurisdiction"`
	PolicyID               string `json:"policy_id"`
	SettlementDenom        string `json:"settlement_denom"`
	PerHolderCap           uint64 `json:"per_holder_cap"`
	TotalSupplyCap         uint64 `json:"total_supply_cap"`
	RedemptionRate         uint64 `json:"redemption_rate"`
	RedemptionDelaySeconds int64  `json:"redemption_delay_seconds"`
	RequireKycHolders      bool   `json:"require_kyc_holders"`
}

func newCreateTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-token [token.json]",
		Short: "Create a new RWA token (issuer admin)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var v createTokenJSON
			if err := readJSON(args[0], &v); err != nil {
				return err
			}
			msg := &types.MsgCreateToken{
				Admin:                  cliCtx.GetFromAddress().String(),
				IssuerId:               v.IssuerID,
				Symbol:                 v.Symbol,
				DisplayName:            v.DisplayName,
				Decimals:               v.Decimals,
				AssetClass:             types.AssetClass(v.AssetClass),
				Jurisdiction:           v.Jurisdiction,
				PolicyId:               v.PolicyID,
				SettlementDenom:        v.SettlementDenom,
				PerHolderCap:           v.PerHolderCap,
				TotalSupplyCap:         v.TotalSupplyCap,
				RedemptionRate:         v.RedemptionRate,
				RedemptionDelaySeconds: v.RedemptionDelaySeconds,
				RequireKycHolders:      v.RequireKycHolders,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type updateTokenJSON struct {
	TokenID                uint64 `json:"token_id"`
	DisplayName            string `json:"display_name"`
	PolicyID               string `json:"policy_id"`
	PerHolderCap           uint64 `json:"per_holder_cap"`
	RedemptionRate         uint64 `json:"redemption_rate"`
	RedemptionDelaySeconds int64  `json:"redemption_delay_seconds"`
	RequireKycHolders      bool   `json:"require_kyc_holders"`
	SetKycFlag             bool   `json:"set_kyc_flag"`
}

func newUpdateTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-token [token.json]",
		Short: "Update mutable fields on an RWA token (issuer admin)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var v updateTokenJSON
			if err := readJSON(args[0], &v); err != nil {
				return err
			}
			msg := &types.MsgUpdateToken{
				Admin:                  cliCtx.GetFromAddress().String(),
				TokenId:                v.TokenID,
				DisplayName:            v.DisplayName,
				PolicyId:               v.PolicyID,
				PerHolderCap:           v.PerHolderCap,
				RedemptionRate:         v.RedemptionRate,
				RedemptionDelaySeconds: v.RedemptionDelaySeconds,
				RequireKycHolders:      v.RequireKycHolders,
				SetKycFlag:             v.SetKycFlag,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// ---- Account flags --------------------------------------------------------

type accountFlagsJSON struct {
	TokenID      uint64 `json:"token_id"`
	Account      string `json:"account"`
	KycCleared   bool   `json:"kyc_cleared"`
	Accredited   bool   `json:"accredited"`
	Jurisdiction string `json:"jurisdiction"`
}

func newSetAccountFlagsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-account-flags [flags.json]",
		Short: "Set per-account compliance flags for a token (issuer admin)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var v accountFlagsJSON
			if err := readJSON(args[0], &v); err != nil {
				return err
			}
			msg := &types.MsgSetAccountFlags{
				Admin:        cliCtx.GetFromAddress().String(),
				TokenId:      v.TokenID,
				Account:      v.Account,
				KycCleared:   v.KycCleared,
				Accredited:   v.Accredited,
				Jurisdiction: v.Jurisdiction,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// ---- Distribution ---------------------------------------------------------

type createDistributionJSON struct {
	TokenID     uint64 `json:"token_id"`
	SnapshotID  uint64 `json:"snapshot_id"`
	TotalAmount uint64 `json:"total_amount"`
	Memo        string `json:"memo"`
}

func newCreateDistributionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-distribution [distribution.json]",
		Short: "Declare a dividend / coupon distribution against a snapshot (issuer admin)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var v createDistributionJSON
			if err := readJSON(args[0], &v); err != nil {
				return err
			}
			msg := &types.MsgCreateDistribution{
				Admin:       cliCtx.GetFromAddress().String(),
				TokenId:     v.TokenID,
				SnapshotId:  v.SnapshotID,
				TotalAmount: v.TotalAmount,
				Memo:        v.Memo,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// ---- Params ---------------------------------------------------------------

type paramsJSON struct {
	MaxIssuers                     uint32 `json:"max_issuers"`
	MaxTokens                      uint32 `json:"max_tokens"`
	MaxHoldersPerSnapshot          uint32 `json:"max_holders_per_snapshot"`
	MaxPendingRedemptionsPerHolder uint32 `json:"max_pending_redemptions_per_holder"`
	MaxLockupsPerHolder            uint32 `json:"max_lockups_per_holder"`
	RequireSanctionsClear          bool   `json:"require_sanctions_clear"`
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Submit a governance proposal to update rwa params",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var v paramsJSON
			if err := readJSON(args[0], &v); err != nil {
				return err
			}
			msg := &types.MsgUpdateParams{
				Authority: cliCtx.GetFromAddress().String(),
				Params: types.Params{
					MaxIssuers:                     v.MaxIssuers,
					MaxTokens:                      v.MaxTokens,
					MaxHoldersPerSnapshot:          v.MaxHoldersPerSnapshot,
					MaxPendingRedemptionsPerHolder: v.MaxPendingRedemptionsPerHolder,
					MaxLockupsPerHolder:            v.MaxLockupsPerHolder,
					RequireSanctionsClear:          v.RequireSanctionsClear,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
