// Package cli provides Cobra commands for the x/meter module. The
// nested-payload flows live here because autocli can't synthesise the
// bytes / enum / nested-struct flag combinations they need
// (RegisterMeteringPoint, UpdateMeteringPoint, SubmitReading,
// SubmitBatch, AuthorizeStream).
package cli

import (
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"energychain/x/meter/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Meter module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterMeteringPointCmd(),
		newUpdateMeteringPointCmd(),
		newSubmitReadingCmd(),
		newSubmitBatchCmd(),
		newAuthorizeStreamCmd(),
	)
	return cmd
}

// newRegisterMeteringPointCmd: tx meter register-metering-point [id] [device-did] [unit] [measurement-type]
//   --grid-zone= --grid-operator= --tariff= --timezone= --metadata-uri=
func newRegisterMeteringPointCmd() *cobra.Command {
	var (
		gridZone, gridOperator, tariff, timezone, metadataURI string
	)
	cmd := &cobra.Command{
		Use:   "register-metering-point [id] [device-did] [unit] [measurement-type]",
		Short: "Register a metering point (signer becomes the owner)",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			mt, err := parseMeasurementType(args[3])
			if err != nil {
				return err
			}
			msg := &types.MsgRegisterMeteringPoint{
				Submitter:       clientCtx.GetFromAddress().String(),
				Id:              args[0],
				DeviceDid:       args[1],
				OwnerAddress:    clientCtx.GetFromAddress().String(),
				Unit:            args[2],
				MeasurementType: mt,
				GridZone:        gridZone,
				GridOperator:    gridOperator,
				Tariff:          tariff,
				Timezone:        timezone,
				MetadataUri:     metadataURI,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().StringVar(&gridZone, "grid-zone", "", "Grid zone (e.g. PJM.WHUB)")
	cmd.Flags().StringVar(&gridOperator, "grid-operator", "", "Grid operator id")
	cmd.Flags().StringVar(&tariff, "tariff", "", "Tariff scheme id")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA timezone id")
	cmd.Flags().StringVar(&metadataURI, "metadata-uri", "", "URI to off-chain metadata document")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newUpdateMeteringPointCmd: tx meter update-metering-point [id]
//   --grid-zone= --grid-operator= --tariff= --unit= --timezone= --metadata-uri=
func newUpdateMeteringPointCmd() *cobra.Command {
	var (
		gridZone, gridOperator, tariff, unit, timezone, metadataURI string
	)
	cmd := &cobra.Command{
		Use:   "update-metering-point [id]",
		Short: "Update mutable fields on a metering point (owner only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			msg := &types.MsgUpdateMeteringPoint{
				Submitter:    clientCtx.GetFromAddress().String(),
				Id:           args[0],
				GridZone:     gridZone,
				GridOperator: gridOperator,
				Tariff:       tariff,
				Unit:         unit,
				Timezone:     timezone,
				MetadataUri:  metadataURI,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().StringVar(&gridZone, "grid-zone", "", "Grid zone")
	cmd.Flags().StringVar(&gridOperator, "grid-operator", "", "Grid operator id")
	cmd.Flags().StringVar(&tariff, "tariff", "", "Tariff scheme id")
	cmd.Flags().StringVar(&unit, "unit", "", "Unit string (e.g. kWh)")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA timezone id")
	cmd.Flags().StringVar(&metadataURI, "metadata-uri", "", "URI to off-chain metadata")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newSubmitReadingCmd: tx meter submit-reading [mp-id] [period] [start] [end] [scheme] [commitment-hex]
//   --plaintext-value=0 --device-signature-hex= --quality=SETTLED
func newSubmitReadingCmd() *cobra.Command {
	var (
		plaintextValue     int64
		deviceSignatureHex string
		qualityFlag        string
	)
	cmd := &cobra.Command{
		Use:   "submit-reading [mp-id] [period] [start] [end] [scheme] [commitment-hex]",
		Short: "Submit a single metered reading",
		Args:  cobra.ExactArgs(6),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			period, err := parsePeriod(args[1])
			if err != nil {
				return err
			}
			start, err := strconv.ParseInt(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("start: %w", err)
			}
			end, err := strconv.ParseInt(args[3], 10, 64)
			if err != nil {
				return fmt.Errorf("end: %w", err)
			}
			scheme, err := parseScheme(args[4])
			if err != nil {
				return err
			}
			cmt, err := hex.DecodeString(args[5])
			if err != nil {
				return fmt.Errorf("commitment hex: %w", err)
			}
			var sig []byte
			if deviceSignatureHex != "" {
				sig, err = hex.DecodeString(deviceSignatureHex)
				if err != nil {
					return fmt.Errorf("device-signature hex: %w", err)
				}
			}
			quality, err := parseQuality(qualityFlag)
			if err != nil {
				return err
			}
			msg := &types.MsgSubmitReading{
				Submitter:        clientCtx.GetFromAddress().String(),
				MeteringPointId:  args[0],
				Period:           period,
				StartTime:        start,
				EndTime:          end,
				CommitmentScheme: scheme,
				CommitmentBytes:  cmt,
				PlaintextValue:   plaintextValue,
				DeviceSignature:  sig,
				DataQuality:      quality,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().Int64Var(&plaintextValue, "plaintext-value", 0, "Optional plaintext value (only valid for PLAINTEXT_SHA256 scheme)")
	cmd.Flags().StringVar(&deviceSignatureHex, "device-signature-hex", "", "Hex-encoded device signature")
	cmd.Flags().StringVar(&qualityFlag, "quality", "SETTLED", "Data quality flag (SETTLED|ESTIMATED|INTERPOLATED|SUSPECT|INVALID)")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newSubmitBatchCmd: tx meter submit-batch [mp-id] [merkle-root-hex] [count] [period] [start] [end] [uri]
//   --device-signature-hex= --merkle-algorithm=sha256
func newSubmitBatchCmd() *cobra.Command {
	var (
		deviceSignatureHex string
		merkleAlgorithm    string
	)
	cmd := &cobra.Command{
		Use:   "submit-batch [mp-id] [merkle-root-hex] [count] [period] [start] [end] [uri]",
		Short: "Submit a Merkle-rooted batch of readings",
		Args:  cobra.ExactArgs(7),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			count, err := strconv.ParseUint(args[2], 10, 32)
			if err != nil {
				return fmt.Errorf("count: %w", err)
			}
			period, err := parsePeriod(args[3])
			if err != nil {
				return err
			}
			start, err := strconv.ParseInt(args[4], 10, 64)
			if err != nil {
				return fmt.Errorf("start: %w", err)
			}
			end, err := strconv.ParseInt(args[5], 10, 64)
			if err != nil {
				return fmt.Errorf("end: %w", err)
			}
			var sig []byte
			if deviceSignatureHex != "" {
				sig, err = hex.DecodeString(deviceSignatureHex)
				if err != nil {
					return fmt.Errorf("device-signature hex: %w", err)
				}
			}
			msg := &types.MsgSubmitBatch{
				Submitter:       clientCtx.GetFromAddress().String(),
				MeteringPointId: args[0],
				MerkleRoot:      args[1],
				MerkleAlgorithm: merkleAlgorithm,
				Count:           uint32(count),
				Period:          period,
				StartTime:       start,
				EndTime:         end,
				Uri:             args[6],
				DeviceSignature: sig,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().StringVar(&deviceSignatureHex, "device-signature-hex", "", "Hex-encoded device signature")
	cmd.Flags().StringVar(&merkleAlgorithm, "merkle-algorithm", types.MerkleAlgoSHA256, "Merkle algorithm tag")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newAuthorizeStreamCmd: tx meter authorize-stream [mp-id] [device-did] [expires-at]
//   --metadata=
func newAuthorizeStreamCmd() *cobra.Command {
	var metadata string
	cmd := &cobra.Command{
		Use:   "authorize-stream [mp-id] [device-did] [expires-at]",
		Short: "Authorize a device to stream readings to a metering point until expires_at (unix sec)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			expires, err := strconv.ParseInt(args[2], 10, 64)
			if err != nil {
				return fmt.Errorf("expires-at: %w", err)
			}
			msg := &types.MsgAuthorizeStream{
				Authorizer:      clientCtx.GetFromAddress().String(),
				MeteringPointId: args[0],
				DeviceDid:       args[1],
				ExpiresAt:       expires,
				Metadata:        metadata,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().StringVar(&metadata, "metadata", "", "Free-form metadata stored with the grant")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// ---------------------------------------------------------------------------
// Enum parsers
// ---------------------------------------------------------------------------

func parsePeriod(s string) (types.ReadingPeriod, error) {
	switch s {
	case "15MIN", "15min", "15m":
		return types.ReadingPeriod_READING_PERIOD_15MIN, nil
	case "1HOUR", "1hour", "1h":
		return types.ReadingPeriod_READING_PERIOD_1HOUR, nil
	case "1DAY", "1day", "1d":
		return types.ReadingPeriod_READING_PERIOD_1DAY, nil
	default:
		return 0, fmt.Errorf("unknown period %q (allowed: 15MIN|1HOUR|1DAY)", s)
	}
}

func parseScheme(s string) (types.CommitmentScheme, error) {
	switch s {
	case "PLAINTEXT_SHA256", "plaintext", "sha256":
		return types.CommitmentScheme_COMMITMENT_SCHEME_PLAINTEXT_SHA256, nil
	case "PEDERSEN_SECP256K1", "pedersen":
		return types.CommitmentScheme_COMMITMENT_SCHEME_PEDERSEN_SECP256K1, nil
	default:
		return 0, fmt.Errorf("unknown commitment scheme %q", s)
	}
}

func parseQuality(s string) (types.DataQualityFlag, error) {
	switch s {
	case "SETTLED":
		return types.DataQualityFlag_DATA_QUALITY_SETTLED, nil
	case "ESTIMATED":
		return types.DataQualityFlag_DATA_QUALITY_ESTIMATED, nil
	case "INTERPOLATED":
		return types.DataQualityFlag_DATA_QUALITY_INTERPOLATED, nil
	case "SUSPECT":
		return types.DataQualityFlag_DATA_QUALITY_SUSPECT, nil
	case "INVALID":
		return types.DataQualityFlag_DATA_QUALITY_INVALID, nil
	default:
		return 0, fmt.Errorf("unknown data quality %q", s)
	}
}

func parseMeasurementType(s string) (types.MeasurementType, error) {
	switch s {
	case "ACTIVE_POWER":
		return types.MeasurementType_MEASUREMENT_TYPE_ACTIVE_POWER, nil
	case "REACTIVE_POWER":
		return types.MeasurementType_MEASUREMENT_TYPE_REACTIVE_POWER, nil
	case "VOLTAGE":
		return types.MeasurementType_MEASUREMENT_TYPE_VOLTAGE, nil
	case "FREQUENCY":
		return types.MeasurementType_MEASUREMENT_TYPE_FREQUENCY, nil
	case "GAS_FLOW":
		return types.MeasurementType_MEASUREMENT_TYPE_GAS_FLOW, nil
	case "HEAT":
		return types.MeasurementType_MEASUREMENT_TYPE_HEAT, nil
	case "OTHER":
		return types.MeasurementType_MEASUREMENT_TYPE_OTHER, nil
	default:
		return 0, fmt.Errorf("unknown measurement type %q", s)
	}
}
