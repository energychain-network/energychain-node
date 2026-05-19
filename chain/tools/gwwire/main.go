// gwwire is a one-shot tool that flips every x/<module>/module.go
// from the stub
//   func (AppModuleBasic) RegisterGRPCGatewayRoutes(_ client.Context, _ *runtime.ServeMux) {}
// to a live wiring that calls the generated
// types.RegisterQueryHandlerClient. It also ensures the file
// imports "context" so the wiring compiles.
//
// Idempotent: re-running after the wiring is in place is a no-op.
// Also fixes the spurious blank line earlier revisions of this tool
// left between "context" and the next stdlib import.
//
// Usage:
//   go run ./tools/gwwire
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	stubPattern = `func (AppModuleBasic) RegisterGRPCGatewayRoutes(_ client.Context, _ *runtime.ServeMux) {}`
	livePattern = `func (AppModuleBasic) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *runtime.ServeMux) {
	if err := types.RegisterQueryHandlerClient(context.Background(), mux, types.NewQueryClient(clientCtx)); err != nil {
		panic(err)
	}
}`
)

func main() {
	matches, err := filepath.Glob("x/*/module.go")
	if err != nil {
		fmt.Fprintln(os.Stderr, "glob:", err)
		os.Exit(1)
	}
	if len(matches) == 0 {
		fmt.Fprintln(os.Stderr, "no x/<module>/module.go files found; run from chain/")
		os.Exit(1)
	}

	changed := 0
	for _, path := range matches {
		bz, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, path, ":", err)
			os.Exit(1)
		}
		src := string(bz)
		orig := src

		if strings.Contains(src, stubPattern) {
			src = strings.Replace(src, stubPattern, livePattern, 1)
		}
		if !strings.Contains(src, `"context"`) {
			src = injectContextImport(src)
		}
		// Fix-up: collapse the spurious blank line that earlier
		// revisions of this tool left between "context" and the
		// next stdlib import.
		src = strings.Replace(src,
			"import (\n\t\"context\"\n\n\t\"encoding/json\"\n",
			"import (\n\t\"context\"\n\t\"encoding/json\"\n",
			1)
		src = strings.Replace(src,
			"import (\n\t\"context\"\n\n\t\"fmt\"\n",
			"import (\n\t\"context\"\n\t\"fmt\"\n",
			1)

		if src == orig {
			fmt.Println("skip   ", path)
			continue
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "write", path, ":", err)
			os.Exit(1)
		}
		fmt.Println("update ", path)
		changed++
	}
	fmt.Printf("%d module.go file(s) updated.\n", changed)
}

// injectContextImport adds "context" to the import block right after
// the opening parenthesis. Assumes a standard gofmt-style block.
func injectContextImport(src string) string {
	const marker = "import (\n"
	idx := strings.Index(src, marker)
	if idx < 0 {
		fmt.Fprintln(os.Stderr, "no import block found; injecting fails")
		os.Exit(1)
	}
	insertAt := idx + len(marker)
	return src[:insertAt] + "\t\"context\"\n" + src[insertAt:]
}
