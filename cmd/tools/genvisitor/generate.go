package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io"

	"go.temporal.io/server/common/log"
)

// generate returns the formatted source for target. dump, when not nil,
// receives the matched path tree for debugging.
func generate(logger log.Logger, target string, dump io.Writer) ([]byte, error) {
	var (
		e   *Emitter
		err error
	)
	switch target {
	case "utf8":
		e = buildUTF8(logger)
	case "payloads":
		e, err = buildPayloads(logger, adminPayloadTables)
	default:
		return nil, fmt.Errorf("unknown -target %q, want utf8 or payloads", target)
	}
	if err != nil {
		return nil, err
	}

	if dump != nil {
		e.root.Dump(dump)
	}

	var buf bytes.Buffer
	if err := e.Generate(&buf); err != nil {
		return nil, err
	}

	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("generated code does not parse: %w", err)
	}

	return src, nil
}
