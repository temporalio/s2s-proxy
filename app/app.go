package app

import (
	"context"

	urcli "github.com/urfave/cli/v2"
	"go.temporal.io/server/common/log"
	"go.uber.org/fx"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/logging"
	"github.com/temporalio/s2s-proxy/proto/compat"
	"github.com/temporalio/s2s-proxy/proxy"
)

const (
	DefaultName    = "s2s-proxy"
	DefaultVersion = "0.0.1" // TODO: configure build-time injection of version.
)

type proxyRunner interface {
	Start() error
	Stop()
}

type App struct {
	name      string
	version   string
	extraOpts []fx.Option
}

func New(name, version string, extraOpts ...fx.Option) *App {
	return &App{name: name, version: version, extraOpts: extraOpts}
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
				&urcli.StringFlag{
					Name:     config.ConfigPathFlag,
					Usage:    "path to proxy config yaml file",
					Required: true,
				},
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
	}

	return app
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
