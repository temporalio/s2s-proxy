package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/proxy"

	"github.com/urfave/cli/v2"
	"go.temporal.io/server/common/log"
	"go.uber.org/fx"
)

const (
	ProxyVersion = "0.0.1"
)

type ProxyParams struct {
	fx.In

	Config config.Config
	Proxy  *proxy.Proxy
}

func run(args []string) error {
	app := buildCLIOptions()
	return app.Run(args)
}

func buildCLIOptions() *cli.App {
	app := cli.NewApp()
	app.Name = "s2s-proxy"
	app.Usage = "Temporal proxy between servers"
	app.Version = ProxyVersion

	app.Commands = []*cli.Command{
		{
			Name:  "start",
			Usage: "Starts the proxy.",
			Flags: []cli.Flag{
				&cli.IntFlag{
					Name:     config.ListenPortFlag,
					Usage:    "grpc port listened by proxy.",
					Aliases:  []string{"l"},
					Required: true,
				},

				&cli.StringFlag{
					Name:     config.RemoteServerRPCAddress,
					Usage:    "remote service address(Host:Port).",
					Aliases:  []string{"r"},
					Required: true,
				},
			},
			Action: startProxy,
		},
	}

	return app
}

func startProxy(c *cli.Context) error {
	var proxyParams ProxyParams

	app := fx.New(
		fx.Provide(func() *cli.Context { return c }),
		fx.Provide(func() log.Logger { return log.NewCLILogger() }),
		config.Module,
		client.Module,
		proxy.Module,
		fx.Populate(&proxyParams),
	)

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
		signal.Stop(c)
	}()

	return ret
}
