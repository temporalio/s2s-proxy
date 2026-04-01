package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/temporalio/s2s-proxy/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// TODO: configure build-time injection of version.
	if err := app.New(app.DefaultName, app.DefaultVersion).Run(ctx, os.Args); err != nil {
		panic(err)
	}
}
