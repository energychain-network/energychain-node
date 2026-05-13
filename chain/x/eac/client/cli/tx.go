// Package cli provides Cobra commands for the x/eac module. Messages
// with structured payloads (RegisterIssuer, UpdateIssuer, IssueBatch,
// SetBridgeAttestation, UpdateParams) live here because autocli
// cannot synthesise the multi-flag list shapes they need.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"energychain/x/eac/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "EAC module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterIssuerCmd(),
		newUpdateIssuerCmd(),
		newIssueBatchCmd(),
		newSetBridgeAttestationCmd(),
		newUpdateParamsCmd(),
	)
	return cmd
}

// ---- Issuer ---------------------------------------------------------------

type issuerJSON struct {
	ID              string  `json:"id"`
	DID             string  `json:"did"`
	DisplayName     string  `json:"display_name"`
	Kinds           []int32 `json:"kinds"`
	IssuerAuthority string  `json:"issuer_authority"`
	Admin           string  `json:"admin"`
}

func readIssuerJSON(path string) (issuerJSON, error) {
	bz, err := os.ReadFile(path)
	if err != nil {
		return issuerJSON{}, fmt.Errorf("read %s: %w", path, err)
	}
	var v issuerJSON
	if err := json.Unmarshal(bz, &v); err != nil {
		return issuerJSON{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return v, nil
}

func newRegisterIssuerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-issuer [issuer.json]",
		Short: "Register a new EAC issuer from a JSON file (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			v, err := readIssuerJSON(args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgRegisterIssuer{
				Authority:       cliCtx.GetFromAddress().String(),
				Id:              v.ID,
				Did:             v.DID,
				DisplayName:     v.DisplayName,
				Kinds:           v.Kinds,
				IssuerAuthority: v.IssuerAuthority,
				Admin:           v.Admin,
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
		Short: "Update an existing EAC issuer from a JSON file (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			v, err := readIssuerJSON(args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgUpdateIssuer{
				Authority:       cliCtx.GetFromAddress().String(),
				Id:              v.ID,
				DisplayName:     v.DisplayName,
				Kinds:           v.Kinds,
				IssuerAuthority: v.IssuerAuthority,
				Admin:           v.Admin,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// ---- Batch ----------------------------------------------------------------

type batchJSON struct {
	IssuerID       string `json:"issuer_id"`
	Kind           int32  `json:"kind"`
	Technology     int32  `json:"technology"`
	ProjectID      string `json:"project_id"`
	DeviceID       string `json:"device_id"`
	GridZone       string `json:"grid_zone"`
	HourStart      int64  `json:"hour_start"`
	HourEnd        int64  `json:"hour_end"`
	VintageYear    uint32 `json:"vintage_year"`
	SourceRegistry string `json:"source_registry"`
	SourceSerial   string `json:"source_serial"`
	Units          uint64 `json:"units"`
	Recipient      string `json:"recipient"`
	PolicyID       string `json:"policy_id"`
}

func newIssueBatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue-batch [batch.json]",
		Short: "Issue a batch of EAC units from a JSON file (issuer authority)",
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
			var b batchJSON
			if err := json.Unmarshal(bz, &b); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			msg := &types.MsgIssueBatch{
				IssuerAuthority: cliCtx.GetFromAddress().String(),
				IssuerId:        b.IssuerID,
				Kind:            types.CertificateKind(b.Kind),
				Technology:      types.Technology(b.Technology),
				ProjectId:       b.ProjectID,
				DeviceId:        b.DeviceID,
				GridZone:        b.GridZone,
				HourStart:       b.HourStart,
				HourEnd:         b.HourEnd,
				VintageYear:     b.VintageYear,
				SourceRegistry:  b.SourceRegistry,
				SourceSerial:    b.SourceSerial,
				Units:           b.Units,
				Recipient:       b.Recipient,
				PolicyId:        b.PolicyID,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// ---- Bridge ---------------------------------------------------------------

func newSetBridgeAttestationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-bridge-attestation [cert-id] [oracle-topic-id] [max-staleness-seconds]",
		Short: "Bind a bridge oracle topic to a certificate (governance only)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var certID uint64
			if _, err := fmt.Sscanf(args[0], "%d", &certID); err != nil {
				return fmt.Errorf("parse cert-id: %w", err)
			}
			var maxStale uint32
			if _, err := fmt.Sscanf(args[2], "%d", &maxStale); err != nil {
				return fmt.Errorf("parse max-staleness-seconds: %w", err)
			}
			msg := &types.MsgSetBridgeAttestation{
				Authority:           cliCtx.GetFromAddress().String(),
				CertificateId:       certID,
				OracleTopicId:       args[1],
				MaxStalenessSeconds: maxStale,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// ---- Params ---------------------------------------------------------------

type paramsJSON struct {
	MaxIssuers            uint32 `json:"max_issuers"`
	MaxKindsPerIssuer     uint32 `json:"max_kinds_per_issuer"`
	MaxUnitsPerBatch      uint32 `json:"max_units_per_batch"`
	HourWindowSeconds     uint32 `json:"hour_window_seconds"`
	RequireSanctionsClear bool   `json:"require_sanctions_clear"`
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Update EAC module params from a JSON file (governance only)",
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
			var p paramsJSON
			if err := json.Unmarshal(bz, &p); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			msg := &types.MsgUpdateParams{
				Authority: cliCtx.GetFromAddress().String(),
				Params: types.Params{
					MaxIssuers:            p.MaxIssuers,
					MaxKindsPerIssuer:     p.MaxKindsPerIssuer,
					MaxUnitsPerBatch:      p.MaxUnitsPerBatch,
					HourWindowSeconds:     p.HourWindowSeconds,
					RequireSanctionsClear: p.RequireSanctionsClear,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
