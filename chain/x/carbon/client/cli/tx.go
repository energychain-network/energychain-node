// Package cli provides Cobra commands for the x/carbon module.
// Messages with structured payloads (RegisterIssuer, UpdateIssuer,
// IssueAllowance, IssueOffset, SetArticle6Status,
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

	"energychain/x/carbon/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Carbon module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterIssuerCmd(),
		newUpdateIssuerCmd(),
		newIssueAllowanceCmd(),
		newIssueOffsetCmd(),
		newSetArticle6StatusCmd(),
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
	Categories      []int32 `json:"categories"`
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
		Short: "Register a new carbon issuer from a JSON file (governance only)",
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
				Categories:      v.Categories,
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
		Short: "Update an existing carbon issuer from a JSON file (governance only)",
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
				Categories:      v.Categories,
				IssuerAuthority: v.IssuerAuthority,
				Admin:           v.Admin,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// ---- Issuance -------------------------------------------------------------

type allowanceJSON struct {
	IssuerID       string `json:"issuer_id"`
	Registry       string `json:"registry"`
	Program        string `json:"program"`
	Jurisdiction   string `json:"jurisdiction"`
	VintageYear    uint32 `json:"vintage_year"`
	SourceRegistry string `json:"source_registry"`
	SourceSerial   string `json:"source_serial"`
	Units          uint64 `json:"units"`
	Recipient      string `json:"recipient"`
	PolicyID       string `json:"policy_id"`
}

func newIssueAllowanceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue-allowance [allowance.json]",
		Short: "Issue a compliance allowance batch (issuer authority)",
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
			var v allowanceJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			msg := &types.MsgIssueAllowance{
				IssuerAuthority: cliCtx.GetFromAddress().String(),
				IssuerId:        v.IssuerID,
				Registry:        v.Registry,
				Program:         v.Program,
				Jurisdiction:    v.Jurisdiction,
				VintageYear:     v.VintageYear,
				SourceRegistry:  v.SourceRegistry,
				SourceSerial:    v.SourceSerial,
				Units:           v.Units,
				Recipient:       v.Recipient,
				PolicyId:        v.PolicyID,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type offsetJSON struct {
	IssuerID               string   `json:"issuer_id"`
	Registry               string   `json:"registry"`
	Program                string   `json:"program"`
	ProjectID              string   `json:"project_id"`
	Methodology            string   `json:"methodology"`
	Jurisdiction           string   `json:"jurisdiction"`
	VintageYear            uint32   `json:"vintage_year"`
	SourceRegistry         string   `json:"source_registry"`
	SourceSerial           string   `json:"source_serial"`
	CCPLabels              []string `json:"ccp_labels"`
	LinkedEACCertificateID uint64   `json:"linked_eac_certificate_id"`
	Article6Status         int32    `json:"article6_status"`
	HostCountry            string   `json:"host_country"`
	RecipientCountry       string   `json:"recipient_country"`
	Units                  uint64   `json:"units"`
	Recipient              string   `json:"recipient"`
	PolicyID               string   `json:"policy_id"`
}

func newIssueOffsetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue-offset [offset.json]",
		Short: "Issue a voluntary offset batch (issuer authority)",
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
			var v offsetJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			msg := &types.MsgIssueOffset{
				IssuerAuthority:        cliCtx.GetFromAddress().String(),
				IssuerId:               v.IssuerID,
				Registry:               v.Registry,
				Program:                v.Program,
				ProjectId:              v.ProjectID,
				Methodology:            v.Methodology,
				Jurisdiction:           v.Jurisdiction,
				VintageYear:            v.VintageYear,
				SourceRegistry:         v.SourceRegistry,
				SourceSerial:           v.SourceSerial,
				CcpLabels:              v.CCPLabels,
				LinkedEacCertificateId: v.LinkedEACCertificateID,
				Article6Status:         types.Article6Status(v.Article6Status),
				HostCountry:            v.HostCountry,
				RecipientCountry:       v.RecipientCountry,
				Units:                  v.Units,
				Recipient:              v.Recipient,
				PolicyId:               v.PolicyID,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// ---- Article 6 ------------------------------------------------------------

type article6JSON struct {
	AssetID          uint64 `json:"asset_id"`
	Status           int32  `json:"status"`
	HostCountry      string `json:"host_country"`
	RecipientCountry string `json:"recipient_country"`
	DocumentURI      string `json:"document_uri"`
	DocumentHash     string `json:"document_hash"`
}

func newSetArticle6StatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-article6-status [a6.json]",
		Short: "Update an OFFSET asset's Article 6 status (issuer admin)",
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
			var v article6JSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			msg := &types.MsgSetArticle6Status{
				Admin:            cliCtx.GetFromAddress().String(),
				AssetId:          v.AssetID,
				Status:           types.Article6Status(v.Status),
				HostCountry:      v.HostCountry,
				RecipientCountry: v.RecipientCountry,
				DocumentUri:      v.DocumentURI,
				DocumentHash:     v.DocumentHash,
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
		Use:   "set-bridge-attestation [asset-id] [oracle-topic-id] [max-staleness-seconds]",
		Short: "Bind a bridge oracle topic to a carbon asset (governance only)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var assetID uint64
			if _, err := fmt.Sscanf(args[0], "%d", &assetID); err != nil {
				return fmt.Errorf("parse asset-id: %w", err)
			}
			var maxStale uint32
			if _, err := fmt.Sscanf(args[2], "%d", &maxStale); err != nil {
				return fmt.Errorf("parse max-staleness-seconds: %w", err)
			}
			msg := &types.MsgSetBridgeAttestation{
				Authority:           cliCtx.GetFromAddress().String(),
				AssetId:             assetID,
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
	MaxIssuers                    uint32 `json:"max_issuers"`
	MaxCategoriesPerIssuer        uint32 `json:"max_categories_per_issuer"`
	MaxUnitsPerBatch              uint32 `json:"max_units_per_batch"`
	MaxCCPLabels                  uint32 `json:"max_ccp_labels"`
	RequireSanctionsClear         bool   `json:"require_sanctions_clear"`
	RequireArticle6ForCrossBorder bool   `json:"require_article6_for_cross_border"`
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Update carbon module params from a JSON file (governance only)",
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
					MaxIssuers:                    p.MaxIssuers,
					MaxCategoriesPerIssuer:        p.MaxCategoriesPerIssuer,
					MaxUnitsPerBatch:              p.MaxUnitsPerBatch,
					MaxCcpLabels:                  p.MaxCCPLabels,
					RequireSanctionsClear:         p.RequireSanctionsClear,
					RequireArticle6ForCrossBorder: p.RequireArticle6ForCrossBorder,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
