// Package cli provides Cobra commands for the x/scheduler module.
// CreateJob / UpdateJob / UpdateParams take a packed-Any payload or
// nested params and live here because autocli cannot synthesise
// the proto.Any shape directly.
package cli

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/spf13/cobra"

	"energychain/x/scheduler/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Scheduler module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newCreateJobCmd(),
		newUpdateJobCmd(),
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

// createJobJSON is the on-disk shape for `scheduler create-job`.
//
// `payload_type_url` + `payload_b64` encode the proto.Any. The
// caller is expected to use `energychaind tx ... --generate-only`
// for the inner Msg, base64-encode the resulting payload bytes,
// and supply the matching type_url. This indirection keeps the
// CLI agnostic to every concrete Msg shape.
type createJobJSON struct {
	PayloadTypeURL  string `json:"payload_type_url"`
	PayloadB64      string `json:"payload_b64"`
	StartTime       int64  `json:"start_time"`
	EndTime         int64  `json:"end_time"`
	IntervalSeconds int64  `json:"interval_seconds"`
	MaxExecutions   uint64 `json:"max_executions"`
	FeeDenom        string `json:"fee_denom"`
	FeePerRun       uint64 `json:"fee_per_run"`
	InitialBudget   uint64 `json:"initial_budget"`
	Memo            string `json:"memo"`
}

func buildAny(typeURL, b64 string) (*cdctypes.Any, error) {
	if typeURL == "" {
		return nil, fmt.Errorf("payload_type_url must be set")
	}
	bz, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("payload_b64 decode: %w", err)
	}
	return &cdctypes.Any{TypeUrl: typeURL, Value: bz}, nil
}

func newCreateJobCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create-job [job.json]",
		Short: "Create a new scheduled job from a JSON descriptor (owner signer)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var v createJobJSON
			if err := readJSON(args[0], &v); err != nil {
				return err
			}
			payload, err := buildAny(v.PayloadTypeURL, v.PayloadB64)
			if err != nil {
				return err
			}
			msg := &types.MsgCreateJob{
				Owner:           cliCtx.GetFromAddress().String(),
				Payload:         payload,
				StartTime:       v.StartTime,
				EndTime:         v.EndTime,
				IntervalSeconds: v.IntervalSeconds,
				MaxExecutions:   v.MaxExecutions,
				FeeDenom:        v.FeeDenom,
				FeePerRun:       v.FeePerRun,
				InitialBudget:   v.InitialBudget,
				Memo:            v.Memo,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type updateJobJSON struct {
	JobID           uint64 `json:"job_id"`
	PayloadTypeURL  string `json:"payload_type_url,omitempty"`
	PayloadB64      string `json:"payload_b64,omitempty"`
	IntervalSeconds int64  `json:"interval_seconds,omitempty"`
	EndTime         int64  `json:"end_time,omitempty"`
	MaxExecutions   uint64 `json:"max_executions,omitempty"`
}

func newUpdateJobCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-job [update.json]",
		Short: "Update a scheduled job (owner signer)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var v updateJobJSON
			if err := readJSON(args[0], &v); err != nil {
				return err
			}
			var payload *cdctypes.Any
			if v.PayloadTypeURL != "" {
				payload, err = buildAny(v.PayloadTypeURL, v.PayloadB64)
				if err != nil {
					return err
				}
			}
			msg := &types.MsgUpdateJob{
				Owner:           cliCtx.GetFromAddress().String(),
				JobId:           v.JobID,
				Payload:         payload,
				IntervalSeconds: v.IntervalSeconds,
				EndTime:         v.EndTime,
				MaxExecutions:   v.MaxExecutions,
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

type paramsJSON struct {
	MaxJobs            uint32 `json:"max_jobs"`
	MaxJobsPerOwner    uint32 `json:"max_jobs_per_owner"`
	MinIntervalSeconds int64  `json:"min_interval_seconds"`
	MaxIntervalSeconds int64  `json:"max_interval_seconds"`
	MaxPayloadBytes    uint32 `json:"max_payload_bytes"`
	MaxJobsPerBlock    uint32 `json:"max_jobs_per_block"`
	DefaultFeeDenom    string `json:"default_fee_denom"`
	FeeCollector       string `json:"fee_collector"`
	ErrorMaxLen        uint32 `json:"error_max_len"`
}

func newUpdateParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [params.json]",
		Short: "Update scheduler module parameters (governance)",
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
					MaxJobs:            v.MaxJobs,
					MaxJobsPerOwner:    v.MaxJobsPerOwner,
					MinIntervalSeconds: v.MinIntervalSeconds,
					MaxIntervalSeconds: v.MaxIntervalSeconds,
					MaxPayloadBytes:    v.MaxPayloadBytes,
					MaxJobsPerBlock:    v.MaxJobsPerBlock,
					DefaultFeeDenom:    v.DefaultFeeDenom,
					FeeCollector:       v.FeeCollector,
					ErrorMaxLen:        v.ErrorMaxLen,
				},
			}
			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
