// The MIT License

// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.

// Copyright (c) 2020 Uber Technologies, Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/temporalio/temporal-proxy/client"
	"github.com/temporalio/temporal-proxy/config"
	"github.com/temporalio/temporal-proxy/proxy"

	"github.com/urfave/cli/v2"
	"go.temporal.io/server/common/log"
	"go.uber.org/fx"
	uberzap "go.uber.org/zap"
)

const (
	ProxyVersion = "0.0.1"
)

type ProxyParams struct {
	fx.In

	Config config.Config
	Proxy  proxy.Proxy
}

// Run executes the saas-agent
func run(args []string) error {
	app := buildCLIOptions()
	return app.Run(args)
}

func buildCLIOptions() *cli.App {
	app := cli.NewApp()
	app.Name = "tempora-proxy"
	app.Usage = "Temporal proxy"
	app.Version = ProxyVersion

	app.Flags = []cli.Flag{
		&cli.IntFlag{
			Name:    config.GRPCPortFlag,
			Usage:   "grpc port listened by proxy.",
			Aliases: []string{"p"},
		},
	}

	app.Commands = []*cli.Command{
		{
			Name:   "start",
			Usage:  "Starts the proxy.",
			Action: startProxy,
		},
	}

	return app
}

func startProxy(c *cli.Context) error {
	var proxyParams ProxyParams

	app := fx.New(
		fx.Provide(func() *cli.Context { return c }),
		fx.Provide(func(zl *uberzap.Logger) log.Logger { return log.NewZapLogger(zl) }),
		config.Module,
		client.Module,
		proxy.Module,
		fx.Populate(&proxyParams),
	)

	// rpcFactory := rpc.NewFactory()
	// server := grpc.NewServer()
	// grpcListener := rpcFactory.GetGRPCListener()
	// proxy := NewProxy(server, grpcListener)
	// proxy.Start()

	if err := app.Err(); err != nil {
		return err
	}

	if err := proxyParams.Proxy.Start(); err != nil {
		return err
	}

	// Waits until interrupt signal from OS arrives
	<-interruptCh()

	proxyParams.Proxy.Stop()
	return nil
}

func main() {
	if err := run(os.Args); err != nil {
		panic(err)
	}
}

// InterruptCh returns channel which will get data when system receives interrupt signal.
func interruptCh() <-chan interface{} {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	ret := make(chan interface{}, 1)
	go func() {
		s := <-c
		ret <- s
		close(ret)
	}()

	return ret
}
