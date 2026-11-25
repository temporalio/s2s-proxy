// Command genchasm writes interceptor/chasm_schema_gen.go, the CHASM schema the
// admin payload visitor decodes CHASM state with.
//
// It is apart from genvisitor because it links the server's CHASM libraries,
// which register many more proto types globally, and genvisitor's targets walk
// the global registry.
package main

import (
	"flag"
	"fmt"
	"go/format"
	"os"

	"go.temporal.io/server/common/log"
)

func main() {
	out := flag.String("out", "", "write to this file instead of stdout; nothing is written if generation fails")
	flag.Parse()

	src, err := generate()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *out == "" {
		_, err = os.Stdout.Write(src)
	} else {
		err = os.WriteFile(*out, src, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// generate returns the formatted source of the CHASM schema.
func generate() ([]byte, error) {
	raw, err := buildChasmSchema(log.NewNoopLogger(), adminChasmTables)
	if err != nil {
		return nil, err
	}

	src, err := format.Source(raw)
	if err != nil {
		return nil, fmt.Errorf("generated code does not parse: %w", err)
	}

	return src, nil
}
