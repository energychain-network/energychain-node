// Package cli wires the x/device module into Cobra. Used for messages
// whose payloads contain nested proto messages that the autocli positional
// synthesizer cannot encode (RegisterDevice, SubmitAttestation, UpdateDevice).
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"

	"energychain/x/device/types"
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Device module transaction subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		newRegisterCmd(),
		newUpdateCmd(),
		newSubmitAttestationCmd(),
	)
	return cmd
}

func loadJSON(path string, dst interface{}) error {
	bz, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return json.Unmarshal(bz, dst)
}

// newRegisterCmd: tx device register <device.json>
//
// device.json mirrors proto Device but `owner_did` is auto-filled from --from.
func newRegisterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register [path/to/device.json]",
		Short: "Register a new device (owner = --from)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var d types.Device
			if err := loadJSON(args[0], &d); err != nil {
				return err
			}
			d.OwnerDid = ctx.GetFromAddress().String()
			msg := &types.MsgRegisterDevice{Owner: d.OwnerDid, Device: d}
			return tx.GenerateOrBroadcastTxCLI(ctx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newUpdateCmd: tx device update <device-did> <update.json>
//
// update.json shape:
//   {
//     "firmware_hash":   "0x...",
//     "grid_zone":       "PJM.WHUB",
//     "lat_e7":          407128000,
//     "lon_e7":         -740060000,
//     "firmware_changed": true
//   }
func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [device-did] [path/to/update.json]",
		Short: "Update mutable device fields",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var u struct {
				FirmwareHash    string `json:"firmware_hash"`
				GridZone        string `json:"grid_zone"`
				LatE7           int64  `json:"lat_e7"`
				LonE7           int64  `json:"lon_e7"`
				FirmwareChanged bool   `json:"firmware_changed"`
			}
			if err := loadJSON(args[1], &u); err != nil {
				return err
			}
			msg := &types.MsgUpdateDevice{
				Owner:           ctx.GetFromAddress().String(),
				DeviceDid:       args[0],
				FirmwareHash:    u.FirmwareHash,
				GridZone:        u.GridZone,
				LatE7:           u.LatE7,
				LonE7:           u.LonE7,
				FirmwareChanged: u.FirmwareChanged,
			}
			return tx.GenerateOrBroadcastTxCLI(ctx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// newSubmitAttestationCmd: tx device submit-attestation <evidence.json>
func newSubmitAttestationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit-attestation [path/to/evidence.json]",
		Short: "Submit attestation evidence (owner-signed)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var ev types.AttestationEvidence
			if err := loadJSON(args[0], &ev); err != nil {
				return err
			}
			msg := &types.MsgSubmitAttestation{Owner: ctx.GetFromAddress().String(), Evidence: ev}
			return tx.GenerateOrBroadcastTxCLI(ctx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
