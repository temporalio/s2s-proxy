package app

import (
	"context"
	"fmt"

	urcli "github.com/urfave/cli/v2"
	"go.temporal.io/server/common/log"
	"go.uber.org/fx"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/logging"
	"github.com/temporalio/s2s-proxy/preflight"
	"github.com/temporalio/s2s-proxy/proto/compat"
	"github.com/temporalio/s2s-proxy/proxy"
)

const (
	DefaultName = "s2s-proxy"
	onlyFlag    = "only"
	outputFlag  = "output"
)

type proxyCheck func(configPath string, version string) preflight.Report

type proxyRunner interface {
	Start() error
	Stop()
}

type App struct {
	name       string
	version    string
	extraOpts  []fx.Option
	checkProxy proxyCheck
}

func New(name, version string, extraOpts ...fx.Option) *App {
	return &App{name: name, version: version, extraOpts: extraOpts, checkProxy: preflight.CheckProxy}
}

func (a *App) Run(ctx context.Context, args []string) error {
	return a.buildApp(ctx).Run(args)
}

type proxyParams struct {
	fx.In
	Runner proxyRunner
}

func (a *App) buildApp(ctx context.Context) *urcli.App {
	app := urcli.NewApp()
	app.Name = a.name
	app.Usage = "Temporal proxy between servers"
	app.Version = a.version

	app.Commands = []*urcli.Command{
		{
			Name:  "start",
			Usage: "Starts the proxy.",
			Flags: []urcli.Flag{
				configPathFlag(),
				&urcli.StringFlag{
					Name:     config.LogLevelFlag,
					Usage:    "Set log level(debug, info, warn, error). Default level is info",
					Required: false,
				},
			},
			Action: func(cliCtx *urcli.Context) error {
				return a.startProxy(ctx, cliCtx)
			},
		},
		{
			Name:  "validate",
			Usage: "Checks the customer-side proxy configuration and local runtime.",
			Flags: []urcli.Flag{
				validationConfigPathFlag(),
				&urcli.StringFlag{
					Name:  onlyFlag,
					Usage: "validation stage to run",
					Value: "proxy",
				},
				&urcli.StringFlag{
					Name:  outputFlag,
					Usage: "output format: text or json",
					Value: "text",
				},
			},
			Action: a.validateProxy,
		},
	}

	return app
}

func configPathFlag() *urcli.StringFlag {
	return &urcli.StringFlag{
		Name:     config.ConfigPathFlag,
		Usage:    "path to proxy config yaml file",
		Required: true,
	}
}

func validationConfigPathFlag() urcli.Flag {
	flag := configPathFlag()
	flag.EnvVars = []string{"CONFIG_YML"}
	return flag
}

func (a *App) validateProxy(cliCtx *urcli.Context) error {
	if only := cliCtx.String(onlyFlag); only != "proxy" {
		return urcli.Exit(fmt.Sprintf("unsupported validation stage %q; phase 1 supports only proxy", only), 2)
	}
	output := cliCtx.String(outputFlag)
	if output != "text" && output != "json" {
		return urcli.Exit(fmt.Sprintf("unsupported output format %q; use text or json", output), 2)
	}

	report := a.checkProxy(cliCtx.String(config.ConfigPathFlag), a.version)
	switch output {
	case "text":
		if err := report.WriteText(cliCtx.App.Writer); err != nil {
			return urcli.Exit(fmt.Sprintf("write validation output: %v", err), 2)
		}
	case "json":
		if err := report.WriteJSON(cliCtx.App.Writer); err != nil {
			return urcli.Exit(fmt.Sprintf("write validation output: %v", err), 2)
		}
	}

	if report.HasFailures() {
		return urcli.Exit("", 1)
	}
	if report.HasUnknowns() {
		return urcli.Exit("", 3)
	}
	return nil
}

func (a *App) startProxy(runCtx context.Context, cliCtx *urcli.Context) error {
	var params proxyParams

	var logCfg log.Config
	if logLevel := cliCtx.String(config.LogLevelFlag); len(logLevel) != 0 {
		logCfg.Level = logLevel
	}

	fxApp := fx.New(
		fx.Provide(func() *urcli.Context { return cliCtx }),
		fx.Provide(func() log.Logger {
			return log.NewZapLogger(log.BuildZapLogger(logCfg))
		}),
		logging.Module,
		config.Module,
		proxy.Module,
		fx.Provide(func(p *proxy.Proxy) proxyRunner { return p }),
		fx.Options(a.extraOpts...),
		fx.Populate(&params),
		fx.Populate(compat.GetCodec().CodecParams),
	)

	if err := fxApp.Err(); err != nil {
		return err
	}

	if err := params.Runner.Start(); err != nil {
		return err
	}

	<-runCtx.Done()
	params.Runner.Stop()
	return nil
}
