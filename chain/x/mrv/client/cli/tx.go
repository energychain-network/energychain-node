// Package cli wires the Cobra commands that are too rich for
// autocli — typically those that take JSON payloads (schemas,
// verifiers, reports, grants) or have enum arguments that
// need string ↔ enum coercion.
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

	"energychain/x/mrv/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "MRV module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterSchemaCmd(),
		newRegisterVerifierCmd(),
		newSubmitReportCmd(),
		newAttestReportCmd(),
		newRejectReportCmd(),
		newGrantViewKeyCmd(),
		newUpdateParamsCmd(),
	)
	return cmd
}

// ---- helpers -----------------------------------------------------------

func parseAssetClass(s string) (types.AssetClass, error) {
	switch strings.ToUpper(s) {
	case "METER":
		return types.AssetClass_ASSET_CLASS_METER, nil
	case "EAC":
		return types.AssetClass_ASSET_CLASS_EAC, nil
	case "CARBON":
		return types.AssetClass_ASSET_CLASS_CARBON, nil
	case "CFE247", "CFE_247":
		return types.AssetClass_ASSET_CLASS_CFE_247, nil
	case "MIXED":
		return types.AssetClass_ASSET_CLASS_MIXED, nil
	}
	return 0, fmt.Errorf("invalid asset_class %q", s)
}

func parseTimeWindow(s string) (types.TimeWindow, error) {
	switch strings.ToUpper(s) {
	case "HOURLY":
		return types.TimeWindow_TIME_WINDOW_HOURLY, nil
	case "DAILY":
		return types.TimeWindow_TIME_WINDOW_DAILY, nil
	case "MONTHLY":
		return types.TimeWindow_TIME_WINDOW_MONTHLY, nil
	case "QUARTERLY":
		return types.TimeWindow_TIME_WINDOW_QUARTERLY, nil
	case "YEARLY":
		return types.TimeWindow_TIME_WINDOW_YEARLY, nil
	case "AD_HOC", "ADHOC":
		return types.TimeWindow_TIME_WINDOW_AD_HOC, nil
	}
	return 0, fmt.Errorf("invalid time_window %q", s)
}

func parseReportFormat(s string) (types.ReportFormat, error) {
	switch strings.ToUpper(s) {
	case "JSON_LD", "JSONLD":
		return types.ReportFormat_REPORT_FORMAT_JSON_LD, nil
	case "XBRL":
		return types.ReportFormat_REPORT_FORMAT_XBRL, nil
	case "ENERGYTAG":
		return types.ReportFormat_REPORT_FORMAT_ENERGYTAG, nil
	case "CSRD_ESRS", "ESRS":
		return types.ReportFormat_REPORT_FORMAT_CSRD_ESRS, nil
	case "CDP":
		return types.ReportFormat_REPORT_FORMAT_CDP, nil
	case "TCFD":
		return types.ReportFormat_REPORT_FORMAT_TCFD, nil
	case "GHG_PROTO", "GHG":
		return types.ReportFormat_REPORT_FORMAT_GHG_PROTO, nil
	case "RE100":
		return types.ReportFormat_REPORT_FORMAT_RE100, nil
	}
	return 0, fmt.Errorf("invalid output_format %q", s)
}

// ---- commands ----------------------------------------------------------

type schemaJSON struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Jurisdiction string `json:"jurisdiction"`
	AssetClass   string `json:"asset_class"`
	TimeWindow   string `json:"time_window"`
	OutputFormat string `json:"output_format"`
	SchemaURI    string `json:"schema_uri"`
	SchemaHash   string `json:"schema_hash"`
}

func newRegisterSchemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-schema [schema.json]",
		Short: "Register a report schema (authority)",
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
			var v schemaJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			ac, err := parseAssetClass(v.AssetClass)
			if err != nil {
				return err
			}
			tw, err := parseTimeWindow(v.TimeWindow)
			if err != nil {
				return err
			}
			of, err := parseReportFormat(v.OutputFormat)
			if err != nil {
				return err
			}
			msg := &types.MsgRegisterSchema{
				Authority:    cliCtx.GetFromAddress().String(),
				Name:         v.Name,
				Version:      v.Version,
				Jurisdiction: v.Jurisdiction,
				AssetClass:   ac,
				TimeWindow:   tw,
				OutputFormat: of,
				SchemaUri:    v.SchemaURI,
				SchemaHash:   v.SchemaHash,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type verifierJSON struct {
	DID                 string   `json:"did"`
	Name                string   `json:"name"`
	SignerAddress       string   `json:"signer_address"`
	AccreditedStandards []string `json:"accredited_standards"`
	Jurisdictions       []string `json:"jurisdictions"`
}

func newRegisterVerifierCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-verifier [verifier.json]",
		Short: "Register an accredited verifier (authority)",
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
			var v verifierJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			msg := &types.MsgRegisterVerifier{
				Authority:           cliCtx.GetFromAddress().String(),
				Did:                 v.DID,
				Name:                v.Name,
				SignerAddress:       v.SignerAddress,
				AccreditedStandards: v.AccreditedStandards,
				Jurisdictions:       v.Jurisdictions,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type reportJSON struct {
	Subject     string `json:"subject"`
	SchemaID    uint64 `json:"schema_id"`
	PeriodStart int64  `json:"period_start"`
	PeriodEnd   int64  `json:"period_end"`
	PayloadURI  string `json:"payload_uri"`
	PayloadHash string `json:"payload_hash"`
	Memo        string `json:"memo"`
	Aggregate   struct {
		MeterWh                  uint64 `json:"meter_wh"`
		EACIssuedWh              uint64 `json:"eac_issued_wh"`
		EACRetiredWh             uint64 `json:"eac_retired_wh"`
		CarbonRetiredMilliTCO2e  uint64 `json:"carbon_retired_milli_tco2e"`
		CFE247HourlyScoreBps     uint32 `json:"cfe247_hourly_score_bps"`
		CFE247AnnualScoreBps     uint32 `json:"cfe247_annual_score_bps"`
		RecordCount              uint64 `json:"record_count"`
	} `json:"aggregate"`
}

func newSubmitReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit-report [report.json]",
		Short: "Submit a draft report",
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
			var v reportJSON
			if err := json.Unmarshal(bz, &v); err != nil {
				return fmt.Errorf("parse %s: %w", args[0], err)
			}
			msg := &types.MsgSubmitReport{
				Submitter:   cliCtx.GetFromAddress().String(),
				Subject:     v.Subject,
				SchemaId:    v.SchemaID,
				PeriodStart: v.PeriodStart,
				PeriodEnd:   v.PeriodEnd,
				PayloadUri:  v.PayloadURI,
				PayloadHash: v.PayloadHash,
				Memo:        v.Memo,
				Aggregate: types.ReportAggregate{
					MeterWh:                 v.Aggregate.MeterWh,
					EacIssuedWh:             v.Aggregate.EACIssuedWh,
					EacRetiredWh:            v.Aggregate.EACRetiredWh,
					CarbonRetiredMilliTco2E: v.Aggregate.CarbonRetiredMilliTCO2e,
					Cfe247HourlyScoreBps:    v.Aggregate.CFE247HourlyScoreBps,
					Cfe247AnnualScoreBps:    v.Aggregate.CFE247AnnualScoreBps,
					RecordCount:             v.Aggregate.RecordCount,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newAttestReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attest-report [verifier-id] [report-id] [verifier-payload-uri] [verifier-payload-hash]",
		Short: "Attest a draft report (verifier)",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			vID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("verifier_id: %w", err)
			}
			rID, err := strconv.ParseUint(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("report_id: %w", err)
			}
			msg := &types.MsgAttestReport{
				VerifierAddress:     cliCtx.GetFromAddress().String(),
				VerifierId:          vID,
				ReportId:            rID,
				VerifierPayloadUri:  args[2],
				VerifierPayloadHash: args[3],
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newRejectReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reject-report [verifier-id] [report-id] [reason]",
		Short: "Reject a draft report (verifier)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			vID, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("verifier_id: %w", err)
			}
			rID, err := strconv.ParseUint(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("report_id: %w", err)
			}
			msg := &types.MsgRejectReport{
				VerifierAddress: cliCtx.GetFromAddress().String(),
				VerifierId:      vID,
				ReportId:        rID,
				Reason:          args[2],
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newGrantViewKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grant-view-key [grantee-did] [report-id] [schema-id] [expires-at]",
		Short: "Grant a view key to a regulator DID (report-id and schema-id are 0 for 'all')",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			rID, err := strconv.ParseUint(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("report_id: %w", err)
			}
			sID, err := strconv.ParseUint(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("schema_id: %w", err)
			}
			exp, err := strconv.ParseInt(args[3], 10, 64)
			if err != nil {
				return fmt.Errorf("expires_at: %w", err)
			}
			msg := &types.MsgGrantViewKey{
				Granter:    cliCtx.GetFromAddress().String(),
				GranteeDid: args[0],
				ReportId:   rID,
				SchemaId:   sID,
				ExpiresAt:  exp,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type paramsJSON struct {
	MaxSchemas                  uint32 `json:"max_schemas"`
	MaxVerifiers                uint32 `json:"max_verifiers"`
	MaxReportsPerSubject        uint32 `json:"max_reports_per_subject"`
	MaxViewKeyGrants            uint32 `json:"max_view_key_grants"`
	MemoMaxLen                  uint32 `json:"memo_max_len"`
	URIMaxLen                   uint32 `json:"uri_max_len"`
	MaxJurisdictionsPerVerifier uint32 `json:"max_jurisdictions_per_verifier"`
	MaxStandardsPerVerifier     uint32 `json:"max_standards_per_verifier"`
	ReasonMaxLen                uint32 `json:"reason_max_len"`
	MaxGrantTTLSeconds          int64  `json:"max_grant_ttl_seconds"`
	MaxGrantsPerBlockSweep      uint32 `json:"max_grants_per_block_sweep"`
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Update module parameters (authority)",
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
					MaxSchemas:                  v.MaxSchemas,
					MaxVerifiers:                v.MaxVerifiers,
					MaxReportsPerSubject:        v.MaxReportsPerSubject,
					MaxViewKeyGrants:            v.MaxViewKeyGrants,
					MemoMaxLen:                  v.MemoMaxLen,
					UriMaxLen:                   v.URIMaxLen,
					MaxJurisdictionsPerVerifier: v.MaxJurisdictionsPerVerifier,
					MaxStandardsPerVerifier:     v.MaxStandardsPerVerifier,
					ReasonMaxLen:                v.ReasonMaxLen,
					MaxGrantTtlSeconds:          v.MaxGrantTTLSeconds,
					MaxGrantsPerBlockSweep:      v.MaxGrantsPerBlockSweep,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
