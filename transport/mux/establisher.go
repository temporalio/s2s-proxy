package mux

import (
	"crypto/tls"
	"errors"
	"math/rand/v2"
	"net"
	"time"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/metrics"
)

type establishingConnProvider struct {
	serverAddress string
	tlsWrapper    func(net.Conn) net.Conn
	logger        log.Logger
	shutdownCheck shutdownCheck
	metricLabels  []string
}

// The connection provider doesn't need the rest of ShutdownOnce, so don't expose it
type shutdownCheck interface {
	IsShutdown() bool
}

var retryPolicy = backoff.NewExponentialRetryPolicy(time.Second).
	WithBackoffCoefficient(1.5).
	WithMaximumInterval(30 * time.Second)

// NewMuxEstablisherProvider makes an outbound call using the provided TCP settings. This constructor handles unpacking
// the TLS config, configures the connection provider with retry and exponential backoff, and sets a disconnect
// sleep time of 1-2 seconds.
func NewMuxEstablisherProvider(name string, transportFn SetTransportCallback, setting config.TCPClientSetting, metricLabels []string, logger log.Logger, shutDown channel.ShutdownOnce) (*MuxProvider, error) {
	tlsWrapper := func(conn net.Conn) net.Conn { return conn }
	if tlsCfg := setting.TLS; tlsCfg.IsEnabled() {
		tlsConfig, err := encryption.GetClientTLSConfig(tlsCfg)
		if err != nil {
			return nil, err
		}
		if tlsConfig == nil {
			return nil, errors.New("tls config was nil even though TLS is enabled")
		}
		tlsWrapper = func(conn net.Conn) net.Conn { return tls.Client(conn, tlsConfig) }
	}
	connPv := &establishingConnProvider{
		serverAddress: setting.ServerAddress,
		tlsWrapper:    tlsWrapper,
		logger:        logger,
		// The connProvider doesn't need the whole shutDown object, so don't give it a reference
		shutdownCheck: shutDown,
		metricLabels:  metricLabels,
	}
	sessionFn := func(conn net.Conn) (*yamux.Session, error) { return yamux.Client(conn, nil) }
	disconnectFn := func() {
		// If the server rapidly disconnects us, we don't want to get caught in a tight loop. Sleep 1-2 seconds before retry
		time.Sleep(time.Second + time.Duration(rand.IntN(1000))*time.Millisecond)
	}
	return NewMuxProvider(name, connPv, sessionFn, disconnectFn, transportFn, metricLabels, logger, shutDown), nil
}

// NewConnection makes a TCP call to establish a connection, then returns it. Retries with backoff over 30 seconds
func (p *establishingConnProvider) NewConnection() (net.Conn, error) {
	var client net.Conn

	dialFn := func() error {
		var err error
		client, err = net.DialTimeout("tcp", p.serverAddress, 5*time.Second)
		if err != nil {
			return err
		}
		client = p.tlsWrapper(client)
		return nil
	}

	onError := func(err error) bool {
		p.logger.Error("mux client failed to dial", tag.Error(err))
		return !p.shutdownCheck.IsShutdown()
	}
	if err := backoff.ThrottleRetry(dialFn, retryPolicy, onError); err != nil {
		p.logger.Error("mux client failed to dial with retry", tag.Error(err))
		metrics.MuxErrors.WithLabelValues(p.metricLabels...).Inc()
		return nil, err
	}
	return client, nil
}

func (p *establishingConnProvider) CloseProvider() {
	// Nothing to close on the client side, we're done.
}
