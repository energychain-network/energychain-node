package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"

	auditm "energychain/x/audit"
	auctionm "energychain/x/auction"
	carbonm "energychain/x/carbon"
	cfe247m "energychain/x/cfe247"
	clearingm "energychain/x/clearing"
	contractm "energychain/x/contract"
	dataslashm "energychain/x/dataslash"
	devicem "energychain/x/device"
	didm "energychain/x/did"
	disputem "energychain/x/dispute"
	eacm "energychain/x/eac"
	escrowm "energychain/x/escrow"
	marketm "energychain/x/market"
	meterm "energychain/x/meter"
	mrvm "energychain/x/mrv"
	oraclem "energychain/x/oracle"
	policym "energychain/x/policy"
	rwam "energychain/x/rwa"
	sanctionsm "energychain/x/sanctions"
	schedulerm "energychain/x/scheduler"
	stablecoinm "energychain/x/stablecoin"
	streampaym "energychain/x/streampay"
)

type basicMod interface {
	DefaultGenesis(codec.JSONCodec) json.RawMessage
	ValidateGenesis(codec.JSONCodec, client.TxEncodingConfig, json.RawMessage) error
}

func main() {
	ir := cdctypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(ir)
	mods := map[string]basicMod{
		"did":        didm.AppModuleBasic{},
		"device":     devicem.AppModuleBasic{},
		"audit":      auditm.AppModuleBasic{},
		"oracle":     oraclem.AppModuleBasic{},
		"meter":      meterm.AppModuleBasic{},
		"policy":     policym.AppModuleBasic{},
		"sanctions":  sanctionsm.AppModuleBasic{},
		"stablecoin": stablecoinm.AppModuleBasic{},
		"eac":        eacm.AppModuleBasic{},
		"carbon":     carbonm.AppModuleBasic{},
		"cfe247":     cfe247m.AppModuleBasic{},
		"rwa":        rwam.AppModuleBasic{},
		"escrow":     escrowm.AppModuleBasic{},
		"scheduler":  schedulerm.AppModuleBasic{},
		"streampay":  streampaym.AppModuleBasic{},
		"contract":   contractm.AppModuleBasic{},
		"auction":    auctionm.AppModuleBasic{},
		"market":     marketm.AppModuleBasic{},
		"clearing":   clearingm.AppModuleBasic{},
		"mrv":        mrvm.AppModuleBasic{},
		"dispute":    disputem.AppModuleBasic{},
		"dataslash":  dataslashm.AppModuleBasic{},
	}

	mode := "dump"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	if mode == "dump" {
		out := map[string]json.RawMessage{}
		for k, m := range mods {
			out[k] = m.DefaultGenesis(cdc)
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return
	}

	// validate <path-to-genesis-template.json>
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: dumpgen validate <genesis_template.json>")
		os.Exit(2)
	}
	raw, err := ioutil.ReadFile(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var doc struct {
		AppState map[string]json.RawMessage `json:"app_state"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		fmt.Fprintln(os.Stderr, "parse:", err)
		os.Exit(2)
	}

	txCfg := client.TxEncodingConfig(nil)
	_ = txCfg

	keys := make([]string, 0, len(mods))
	for k := range mods {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	failed := 0
	for _, k := range keys {
		m := mods[k]
		section, ok := doc.AppState[k]
		if !ok {
			fmt.Printf("MISSING  %-12s no app_state.%s section\n", k, k)
			failed++
			continue
		}
		if err := m.ValidateGenesis(cdc, nil, section); err != nil {
			fmt.Printf("FAIL     %-12s %v\n", k, err)
			failed++
			continue
		}
		fmt.Printf("OK       %-12s\n", k)
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "%d module(s) failed validation\n", failed)
		os.Exit(1)
	}
	fmt.Println("all 22 native modules: ValidateGenesis OK")
}
