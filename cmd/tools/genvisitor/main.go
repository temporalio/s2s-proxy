package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	// Import to populate the protoregistry
	_ "go.temporal.io/api/workflowservice/v1"
	_ "go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
)

func main() {
	debugFlag := flag.Bool("debug", false, "enable debug logs to stderr")
	dumpTree := flag.Bool("dump-tree", false, "print the tree of matched paths in the type hierarchy to stderr")
	target := flag.String("target", "utf8", "what to generate: utf8 or payloads")
	out := flag.String("out", "", "write to this file instead of stdout; nothing is written if generation fails")
	flag.Parse()

	var logger log.Logger
	if *debugFlag {
		logger = log.NewCLILogger()
	} else {
		logger = log.NewNoopLogger()
	}

	var dump io.Writer
	if *dumpTree {
		dump = os.Stderr
	}

	src, err := generate(logger, *target, dump)
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
