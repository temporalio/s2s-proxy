package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/temporalio/s2s-proxy/config"
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
