package mux

import (
	"crypto/tls"
	"errors"
	"net"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/metrics"
)

// NewMuxReceiverProvider runs a TCP server and waits for a client to connect. Once a connection is established and
// authenticated with the TLS config, it starts a yamux session and returns the details using transportFn
func NewMuxReceiverProvider(name string, transportFn SetTransportCallback, setting config.TCPServerSetting, metricLabels []string, upstreamLog log.Logger) (*MuxProvider, error) {
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
		tlsWrapper = func(conn net.Conn) net.Conn { return tls.Client(conn, tlsConfig) }
	}
	listener, err := net.Listen("tcp", setting.ListenAddress)
	if err != nil {
		return nil, err
	}
	connPv := &receivingConnProvider{listener, tlsWrapper, logger, metricLabels}
	sessionFn := func(conn net.Conn) (*yamux.Session, error) { return yamux.Server(conn, nil) }
	disconnectFn := func() {}
	return NewMuxProvider(name, connPv, sessionFn, disconnectFn, transportFn, metricLabels, logger, channel.NewShutdownOnce()), nil
}

type receivingConnProvider struct {
	listener     net.Listener
	tlsWrapper   func(net.Conn) net.Conn
	logger       log.Logger
	metricLabels []string
}

func (r *receivingConnProvider) GetConnection() (net.Conn, error) {
	conn, err := r.listener.Accept()
	if err != nil {
		r.logger.Fatal("listener.Accept failed", tag.Error(err))
		metrics.MuxErrors.WithLabelValues(r.metricLabels...).Inc()
		return nil, err
	}
	r.logger.Info("Accept new connection", tag.NewStringTag("remoteAddr", conn.RemoteAddr().String()))
	return r.tlsWrapper(conn), nil
}

func (r *receivingConnProvider) CloseProvider() {
	err := r.listener.Close()
	if err != nil {
		r.logger.Fatal("listener.Close failed", tag.Error(err))
	}
}
