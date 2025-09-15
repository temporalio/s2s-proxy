package mux

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/metrics"
)

// This file contains implementation details for the "receiving" MuxProvider. It uses a TCP server to establish
// a yamux session in response to an inbound TCP call.
// This file contains logic only, goroutine and control flow is handled in provider.go

type receivingConnProvider struct {
	listener       net.Listener
	tlsWrapper     func(net.Conn) net.Conn
	logger         log.Logger
	metricLabels   []string
	shouldShutDown func() bool
	hasCleanedUp   channel.ShutdownOnce
}

// NewMuxReceiverProvider runs a TCP server and waits for a client to connect. Once a connection is established and
// authenticated with the TLS config, it starts a yamux session and returns the details using transportFn
func NewMuxReceiverProvider(name string, transportFn SetTransportCallback, setting config.TCPServerSetting, metricLabels []string, upstreamLog log.Logger, shouldShutDown channel.ShutdownOnce) (MuxProvider, error) {
	logger := log.With(upstreamLog, tag.NewStringTag("component", "receivingMux"), tag.NewStringTag("listenAddr", setting.ListenAddress))
	tlsWrapper := func(conn net.Conn) net.Conn { return conn }
	if tlsCfg := setting.TLS; tlsCfg.IsEnabled() {
		tlsConfig, err := encryption.GetServerTLSConfig(tlsCfg, logger)
		if err != nil {
			return nil, err
		}
		if tlsConfig == nil {
			return nil, errors.New("tls config was nil even though TLS is enabled")
		}
		tlsWrapper = func(conn net.Conn) net.Conn { return tls.Server(conn, tlsConfig) }
	}
	listener, err := net.Listen("tcp", setting.ListenAddress)
	if err != nil {
		return nil, err
	}
	connPv := &receivingConnProvider{
		listener:       listener,
		tlsWrapper:     tlsWrapper,
		logger:         logger,
		metricLabels:   metricLabels,
		shouldShutDown: func() bool { return shouldShutDown.IsShutdown() },
		hasCleanedUp:   channel.NewShutdownOnce(),
	}
	go func() {
		<-shouldShutDown.Channel()
		err := listener.Close()
		if err != nil {
			logger.Fatal("listener.Close failed", tag.Error(err))
		}
		connPv.hasCleanedUp.Shutdown()
	}()
	sessionFn := func(conn net.Conn) (*yamux.Session, error) {
		cfg := yamux.DefaultConfig()
		cfg.Logger = wrapLoggerForYamux{logger: logger}
		cfg.LogOutput = nil
		return yamux.Server(conn, cfg)
	}
	disconnectFn := func() {}
	return NewMuxProvider(name, connPv, sessionFn, disconnectFn, transportFn, metricLabels, logger, shouldShutDown), nil
}

// NewConnection waits on the TCP server for a connection, then provides it
func (r *receivingConnProvider) NewConnection() (net.Conn, error) {
	conn, err := r.listener.Accept()
	// Log a nicer message when shutting down normally
	if r.shouldShutDown() {
		r.logger.Info("Listener cancelled due to shutdown")
		return nil, fmt.Errorf("provider shutting down")
	}
	if err != nil {
		r.logger.Fatal("listener.Accept failed", tag.Error(err))
		metrics.MuxErrors.WithLabelValues(r.metricLabels...).Inc()
		return nil, err
	}
	r.logger.Info("Accept new connection", tag.NewStringTag("remoteAddr", conn.RemoteAddr().String()))
	return r.tlsWrapper(conn), nil
}

func (r *receivingConnProvider) CloseCh() <-chan struct{} {
	return r.hasCleanedUp.Channel()
}
