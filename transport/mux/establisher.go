package mux

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/metrics"
)

// This file contains implementation details for the "establishing" MuxProvider. It uses a TCP client to establish
// a yamux session using provided TCP client settings.
// This file contains logic only, goroutine and control flow is handled in provider.go

type establishingConnProvider struct {
	serverAddress string
	tlsWrapper    func(net.Conn) net.Conn
	logger        log.Logger
	lifetime      context.Context
	metricLabels  []string
}

var (
	retryPolicy = backoff.NewExponentialRetryPolicy(time.Second).
			WithBackoffCoefficient(1.5).
			WithMaximumInterval(30 * time.Second)

	// The establisher provider never has cleanup work, so we provide the same closed channel on CloseCh()
	alwaysClosedCh = make(chan struct{})
)

func init() {
	close(alwaysClosedCh)
}

// NewMuxEstablisherProvider makes an outbound call using the provided TCP settings. This constructor handles unpacking
// the TLS config, configures the connection provider with retry and exponential backoff, and sets a disconnect
// sleep time of 1-2 seconds.
func NewMuxEstablisherProvider(lifetime context.Context, name string, transportFn AddNewMux, connectionsCapacity int64, setting config.TCPTLSInfo, metricLabels []string, logger log.Logger) (MuxProvider, error) {
	tlsWrapper := func(conn net.Conn) net.Conn { return conn }
	if tlsCfg := setting.TLSConfig; tlsCfg.IsEnabled() {
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
		serverAddress: setting.ConnectionString,
		tlsWrapper:    tlsWrapper,
		logger:        logger,
		// The connProvider doesn't need the whole shutDown object, so don't give it a reference
		lifetime:     lifetime,
		metricLabels: metricLabels,
	}
	sessionFn := func(conn net.Conn) (*yamux.Session, error) {
		cfg := yamux.DefaultConfig()
		cfg.Logger = wrapLoggerForYamux{logger: logger}
		cfg.LogOutput = nil
		cfg.StreamCloseTimeout = 30 * time.Second
		return yamux.Client(conn, cfg)
	}
	// pre-initialize the MuxDial metrics
	metrics.MuxDialFailed.WithLabelValues(metricLabels...)
	metrics.MuxDialSuccess.WithLabelValues(metricLabels...)
	return NewMuxProvider(lifetime, name, connPv, sessionFn, connectionsCapacity, transportFn, metricLabels, logger), nil
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

	retryable := func(err error) bool {
		if p.lifetime.Err() != nil {
			// Shutting down, just exit
			return false
		}
		p.logger.Info("mux client failed to dial", tag.Error(err))
		return true
	}
	if err := backoff.ThrottleRetry(dialFn, retryPolicy, retryable); err != nil {
		if p.lifetime.Err() != nil {
			// shutting down, just exit
			return nil, p.lifetime.Err()
		}
		p.logger.Error("mux client failed to dial with retry", tag.Error(err))
		metrics.MuxDialFailed.WithLabelValues(p.metricLabels...).Inc()
		return nil, err
	}
	metrics.MuxDialSuccess.WithLabelValues(p.metricLabels...).Inc()
	return client, nil
}

// CloseCh for the establisher is a no-op because there are no long-lived objects owned by it
func (p *establishingConnProvider) CloseCh() <-chan struct{} {
	return alwaysClosedCh
}

func (p *establishingConnProvider) Address() string {
	return p.serverAddress
}
