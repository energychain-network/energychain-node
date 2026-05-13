// Package cli provides Cobra commands for the x/escrow module.
// CreateEscrow / Approve / UpdateParams take JSON / structured
// payloads that autocli cannot synthesise directly, so they live
// here.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"energychain/x/escrow/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Escrow module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newCreateEscrowCmd(),
		newApproveCmd(),
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

// createEscrowJSON is the on-disk shape for `escrow create-escrow`.
// Field names match the proto field names (snake_case) so the same
// payload can be reused with grpc tooling.
type createEscrowJSON struct {
	Beneficiary       string         `json:"beneficiary"`
	FallbackAddr      string         `json:"fallback_addr"`
	Committee         []string       `json:"committee"`
	ApprovalThreshold uint32         `json:"approval_threshold"`
	Arbiter           string         `json:"arbiter,omitempty"`
	Kind              string         `json:"kind"`
	StablecoinDenom   string         `json:"stablecoin_denom,omitempty"`
	RWATokenID        uint64         `json:"rwa_token_id,omitempty"`
	Amount            uint64         `json:"amount"`
	Triggers          types.Triggers `json:"triggers"`
	Memo              string         `json:"memo,omitempty"`
}

func parseAssetKind(s string) (types.AssetKind, error) {
	switch s {
	case "stablecoin", "STABLECOIN":
		return types.AssetKind_ASSET_KIND_STABLECOIN, nil
	case "rwa", "RWA":
		return types.AssetKind_ASSET_KIND_RWA, nil
	}
	return types.AssetKind_ASSET_KIND_UNSPECIFIED, fmt.Errorf("unknown kind %q (expected stablecoin|rwa)", s)
}

func parseIntent(s string) (types.Intent, error) {
	switch s {
	case "release", "RELEASE":
		return types.Intent_INTENT_RELEASE, nil
	case "refund", "REFUND":
		return types.Intent_INTENT_REFUND, nil
	}
	return types.Intent_INTENT_UNSPECIFIED, fmt.Errorf("unknown intent %q (expected release|refund)", s)
}

func newCreateEscrowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-escrow [escrow.json]",
		Short: "Create a new escrow from a JSON descriptor (depositor signer)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var v createEscrowJSON
			if err := readJSON(args[0], &v); err != nil {
				return err
			}
			kind, err := parseAssetKind(v.Kind)
			if err != nil {
				return err
			}
			msg := &types.MsgCreateEscrow{
				Depositor:         cliCtx.GetFromAddress().String(),
				Beneficiary:       v.Beneficiary,
				FallbackAddr:      v.FallbackAddr,
				Committee:         v.Committee,
				ApprovalThreshold: v.ApprovalThreshold,
				Arbiter:           v.Arbiter,
				Kind:              kind,
				StablecoinDenom:   v.StablecoinDenom,
				RwaTokenId:        v.RWATokenID,
				Amount:            v.Amount,
				Triggers:          v.Triggers,
				Memo:              v.Memo,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newApproveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve [escrow-id] [release|refund] [memo]",
		Short: "Approve an escrow's release or refund (committee signer)",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var id uint64
			if _, err := fmt.Sscanf(args[0], "%d", &id); err != nil {
				return fmt.Errorf("parse escrow id: %w", err)
			}
			intent, err := parseIntent(args[1])
			if err != nil {
				return err
			}
			memo := ""
			if len(args) > 2 {
				memo = args[2]
			}
			msg := &types.MsgApprove{
				Signer:   cliCtx.GetFromAddress().String(),
				EscrowId: id,
				Intent:   intent,
				Memo:     memo,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type paramsJSON struct {
	MaxEscrows                 uint32 `json:"max_escrows"`
	MaxSignersPerEscrow        uint32 `json:"max_signers_per_escrow"`
	MaxReleaseHorizonSeconds   int64  `json:"max_release_horizon_seconds"`
	RequireSanctionsClear      bool   `json:"require_sanctions_clear"`
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Update escrow module parameters (governance)",
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
					MaxEscrows:                v.MaxEscrows,
					MaxSignersPerEscrow:       v.MaxSignersPerEscrow,
					MaxReleaseHorizonSeconds:  v.MaxReleaseHorizonSeconds,
					RequireSanctionsClear:     v.RequireSanctionsClear,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
