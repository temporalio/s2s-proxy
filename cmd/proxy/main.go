package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/temporalio/s2s-proxy/app"
)

// Version is injected at link time via -ldflags "-X main.Version=x.y.z".
// Three cases:
//   - local: "dev"             local build (Makefile default, no VERSION arg).
//   - dispatch: "dev-<sha>"    CI build on main push; set by docker.yaml. Or manual dispatch with a commit SHA.
//   - release: "v1.2.3"        GitHub release; set to the release tag by docker.yaml.
var Version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.New(app.DefaultName, Version).Run(ctx, os.Args); err != nil {
		panic(err)
	}
}
