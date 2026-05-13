// Package cli provides Cobra commands for x/cfe247 messages whose
// shape doesn't fit autocli's positional-only forms (RegisterDataProvider,
// UpdateDataProvider, RegisterGridZone, AttestHourlyConsumptionBatch,
// GenerateReport, UpdateParams).
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"energychain/x/cfe247/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "CFE 24/7 module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterDataProviderCmd(),
		newUpdateDataProviderCmd(),
		newRegisterGridZoneCmd(),
		newAttestBatchCmd(),
		newGenerateReportCmd(),
		newUpdateParamsCmd(),
	)
	return cmd
}

type providerJSON struct {
	ID          string `json:"id"`
	DID         string `json:"did"`
	DisplayName string `json:"display_name"`
	Attestor    string `json:"attestor"`
	Admin       string `json:"admin"`
}

func readJSON[T any](path string) (T, error) {
	var v T
	bz, err := os.ReadFile(path)
	if err != nil {
		return v, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(bz, &v); err != nil {
		return v, fmt.Errorf("parse %s: %w", path, err)
	}
	return v, nil
}

func newRegisterDataProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-data-provider [provider.json]",
		Short: "Register a new data provider (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			v, err := readJSON[providerJSON](args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgRegisterDataProvider{
				Authority: cliCtx.GetFromAddress().String(),
				Id: v.ID, Did: v.DID, DisplayName: v.DisplayName,
				Attestor: v.Attestor, Admin: v.Admin,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func newUpdateDataProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-data-provider [provider.json]",
		Short: "Update a data provider (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			v, err := readJSON[providerJSON](args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgUpdateDataProvider{
				Authority: cliCtx.GetFromAddress().String(),
				Id: v.ID, DisplayName: v.DisplayName,
				Attestor: v.Attestor, Admin: v.Admin,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type zoneJSON struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Country     string `json:"country"`
	Description string `json:"description"`
}

func newRegisterGridZoneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-grid-zone [zone.json]",
		Short: "Register a grid zone (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			v, err := readJSON[zoneJSON](args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgRegisterGridZone{
				Authority: cliCtx.GetFromAddress().String(),
				Id: v.ID, DisplayName: v.DisplayName,
				Country: v.Country, Description: v.Description,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type batchJSON struct {
	DataProviderID string                   `json:"data_provider_id"`
	Entries        []types.ConsumptionEntry `json:"entries"`
}

func newAttestBatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attest-batch [batch.json]",
		Short: "Submit a batch of hourly consumption attestations (data provider attestor)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			v, err := readJSON[batchJSON](args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgAttestHourlyConsumptionBatch{
				Attestor:       cliCtx.GetFromAddress().String(),
				DataProviderId: v.DataProviderID,
				Entries:        v.Entries,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type reportJSON struct {
	SubjectID   string `json:"subject_id"`
	Format      int32  `json:"format"`
	PeriodStart int64  `json:"period_start"`
	PeriodEnd   int64  `json:"period_end"`
	ReportURI   string `json:"report_uri"`
	ReportHash  string `json:"report_hash"`
}

func newGenerateReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate-report [report.json]",
		Short: "Generate a report package (subject admin)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			v, err := readJSON[reportJSON](args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgGenerateReport{
				Admin:     cliCtx.GetFromAddress().String(),
				SubjectId: v.SubjectID,
				Format:    types.ReportFormat(v.Format),
				PeriodStart: v.PeriodStart, PeriodEnd: v.PeriodEnd,
				ReportUri: v.ReportURI, ReportHash: v.ReportHash,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type paramsJSON struct {
	MaxDataProviders        uint32 `json:"max_data_providers"`
	MaxGridZones            uint32 `json:"max_grid_zones"`
	MaxSubjects             uint32 `json:"max_subjects"`
	MaxAttestationsPerBatch uint32 `json:"max_attestations_per_batch"`
	MaxMatchesPerCall       uint32 `json:"max_matches_per_call"`
	MaxReportPeriodDays     uint32 `json:"max_report_period_days"`
	RequireActiveProvider   bool   `json:"require_active_provider"`
	HourSeconds             uint32 `json:"hour_seconds"`
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Update cfe247 module params (governance only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			v, err := readJSON[paramsJSON](args[0])
			if err != nil {
				return err
			}
			msg := &types.MsgUpdateParams{
				Authority: cliCtx.GetFromAddress().String(),
				Params: types.Params{
					MaxDataProviders:        v.MaxDataProviders,
					MaxGridZones:            v.MaxGridZones,
					MaxSubjects:             v.MaxSubjects,
					MaxAttestationsPerBatch: v.MaxAttestationsPerBatch,
					MaxMatchesPerCall:       v.MaxMatchesPerCall,
					MaxReportPeriodDays:     v.MaxReportPeriodDays,
					RequireActiveProvider:   v.RequireActiveProvider,
					HourSeconds:             v.HourSeconds,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
