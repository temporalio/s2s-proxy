package proxy

import (
	"github.com/urfave/cli/v2"
)

const (
	RemoteAddressFlag = "remote-address"
	ProxyVersion      = "0.0.1"
)

// Run executes the saas-agent
func Run(args []string) error {
	app := buildCLIOptions()
	return app.Run(args)
}

func buildCLIOptions() *cli.App {
	app := cli.NewApp()
	app.Name = "tempora-proxy"
	app.Usage = "Temporal proxy"
	app.Version = ProxyVersion

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    RemoteAddressFlag,
			Aliases: []string{"ra"},
		},
	}

	app.Commands = []*cli.Command{
		{
			Name:   "start",
			Usage:  "Starts the worker. Will block indefinitely",
			Action: startProxy,
		},
	}

	return app
}

func startProxy(c *cli.Context) error {
	return nil
}
