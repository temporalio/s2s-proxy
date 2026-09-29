package app

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	urcli "github.com/urfave/cli/v2"
	"go.uber.org/fx"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/preflight"
)

type stubbedProxy struct {
	startCalled bool
	startErr    error
	stopped     bool
}

func (s *stubbedProxy) Start() error { s.startCalled = true; return s.startErr }
func (s *stubbedProxy) Stop()        { s.stopped = true }

type stubConfigProvider struct{}

func (s *stubConfigProvider) GetS2SProxyConfig() config.S2SProxyConfig {
	return config.S2SProxyConfig{}
}

func withStubbedConfig() fx.Option {
	return fx.Decorate(func() config.ConfigProvider { return &stubConfigProvider{} })
}

func withStubbedProxy(stub *stubbedProxy) fx.Option {
	return fx.Decorate(func() proxyRunner { return stub })
}

func runWithCancelledCtx(t *testing.T, a *App) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return a.Run(ctx, []string{"app", "start", "--" + config.ConfigPathFlag, "stubbed.yaml"})
}

const testVersion = "dev"

func TestBuildApp_ReflectsNameAndVersion(t *testing.T) {
	app := New("just-another-proxy-💁", "9.9.9").buildApp(context.Background())
	assert.Equal(t, "just-another-proxy-💁", app.Name)
	assert.Equal(t, "9.9.9", app.Version)
}

func TestBuildApp_IncludesValidateCommand(t *testing.T) {
	app := New(DefaultName, testVersion).buildApp(context.Background())
	require.NotNil(t, app.Command("validate"))
}

func TestRunValidate_UsesConfigEnvironmentAndWritesReport(t *testing.T) {
	t.Setenv("CONFIG_YML", "/config/config.yaml")
	var checkedPath string
	a := New(DefaultName, testVersion)
	a.checkProxy = func(configPath string, version string) preflight.Report {
		checkedPath = configPath
		return preflight.Report{
			ConfigPath:   configPath,
			ConfigSHA256: "abc123",
			Version:      version,
			Pod:          "proxy-pod-0",
			Results:      []preflight.Result{{Status: preflight.StatusPass, Name: "config file parses"}},
		}
	}
	cliApp := a.buildApp(context.Background())
	var output bytes.Buffer
	cliApp.Writer = &output

	require.NoError(t, cliApp.Run([]string{"app", "validate", "--only", "proxy"}))
	assert.Equal(t, "/config/config.yaml", checkedPath)
	assert.Contains(t, output.String(), "config file parses")
}

func TestValidateProxy_ExitCodes(t *testing.T) {
	tests := []struct {
		name        string
		stage       string
		output      string
		results     []preflight.Result
		wantCode    int
		wantNoError bool
	}{
		{
			name:        "success",
			stage:       "proxy",
			output:      "text",
			results:     []preflight.Result{{Status: preflight.StatusPass, Name: "good"}},
			wantNoError: true,
		},
		{
			name:     "failed check",
			stage:    "proxy",
			output:   "text",
			results:  []preflight.Result{{Status: preflight.StatusFail, Name: "bad"}},
			wantCode: 1,
		},
		{
			name:     "incomplete check",
			stage:    "proxy",
			output:   "json",
			results:  []preflight.Result{{Status: preflight.StatusUnknown, Name: "not observed"}},
			wantCode: 3,
		},
		{
			name:     "unsupported stage",
			stage:    "cluster",
			output:   "text",
			wantCode: 2,
		},
		{
			name:     "unsupported output",
			stage:    "proxy",
			output:   "xml",
			wantCode: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(DefaultName, testVersion)
			a.checkProxy = func(configPath string, version string) preflight.Report {
				return preflight.Report{ConfigPath: configPath, Version: version, Results: tt.results}
			}
			cliCtx := validationContext(t, tt.stage, tt.output)
			err := a.validateProxy(cliCtx)
			if tt.wantNoError {
				require.NoError(t, err)
				return
			}
			var exitCoder urcli.ExitCoder
			require.ErrorAs(t, err, &exitCoder)
			assert.Equal(t, tt.wantCode, exitCoder.ExitCode())
		})
	}
}

func TestRun_MissingConfigFlag_ReturnsError(t *testing.T) {
	err := New(DefaultName, testVersion).Run(
		context.Background(),
		[]string{"app", "start"}, // no --config flag
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config")
}

func TestRun_BadConfigPath_ReturnsError(t *testing.T) {
	// tries to read the file w/o stubbed config, so it fails.
	err := New(DefaultName, testVersion).Run(
		context.Background(),
		[]string{"app", "start", "--" + config.ConfigPathFlag, "/not/stubbed.yaml"},
	)
	require.Error(t, err)
}

func TestRun_CancelledContext_StartsAndStopsProxy(t *testing.T) {
	stub := &stubbedProxy{}
	err := runWithCancelledCtx(t, New(DefaultName, testVersion, withStubbedConfig(), withStubbedProxy(stub)))
	require.NoError(t, err)
	assert.True(t, stub.startCalled)
	assert.True(t, stub.stopped)
}

func TestRun_ProxyStartError_PropagatesAndSkipsStop(t *testing.T) {
	stub := &stubbedProxy{startErr: errors.New("start failed")}
	err := New(DefaultName, testVersion, withStubbedConfig(), withStubbedProxy(stub)).Run(
		context.Background(),
		[]string{"app", "start", "--" + config.ConfigPathFlag, "stubbed.yaml"},
	)
	require.ErrorContains(t, err, "start failed")
	assert.False(t, stub.stopped)
}

func TestRun_ExtraOptsSeam_InjectedRunnerIsUsed(t *testing.T) {
	stub := &stubbedProxy{}
	err := runWithCancelledCtx(t, New(DefaultName, testVersion, withStubbedConfig(), withStubbedProxy(stub)))
	require.NoError(t, err)
	assert.True(t, stub.startCalled)
}

func TestRun_ContextCancelledMidRun_StopsProxy(t *testing.T) {
	stub := &stubbedProxy{}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- New(DefaultName, testVersion, withStubbedConfig(), withStubbedProxy(stub)).Run(
			ctx,
			[]string{"app", "start", "--" + config.ConfigPathFlag, "stubbed.yaml"},
		)
	}()

	cancel()
	require.NoError(t, <-done)
	assert.True(t, stub.stopped)
}

func TestRun_LogLevelFlag_ParsedWithoutError(t *testing.T) {
	stub := &stubbedProxy{}
	err := runWithCancelledCtx(t, New(DefaultName, testVersion, withStubbedConfig(), withStubbedProxy(stub)))
	require.NoError(t, err)
}

func validationContext(t *testing.T, stage string, output string) *urcli.Context {
	t.Helper()
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.String(config.ConfigPathFlag, "/config/config.yaml", "")
	flags.String(onlyFlag, "proxy", "")
	flags.String(outputFlag, "text", "")
	require.NoError(t, flags.Set(onlyFlag, stage))
	require.NoError(t, flags.Set(outputFlag, output))
	cliApp := urcli.NewApp()
	cliApp.Writer = &bytes.Buffer{}
	return urcli.NewContext(cliApp, flags, nil)
}
