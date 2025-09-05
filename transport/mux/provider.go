package mux

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	// MuxProvider manages the process of opening a connection with connProvider, setting up a yamux Session with sessionFn,
	// and then reporting that session via setNewTransport. If the session closes, a new one will be created and notified
	// using setNewTransport.
	MuxProvider struct {
		name            string
		connProvider    connProvider
		sessionFn       func(net.Conn) (*yamux.Session, error)
		onDisconnectFn  func()
		setNewTransport SetTransportCallback
		metricLabels    []string
		logger          log.Logger
		shutDown        context.Context
		reportFatal     context.CancelCauseFunc
		startOnce       sync.Once
	}
	SetTransportCallback func(session *yamux.Session, conn net.Conn)
	// connProvider represents a way to get connections, either as a client or a server. MuxProvider guarantees that
	// Close is called when the provider exits
	connProvider interface {
		GetConnection() (net.Conn, error)
		CloseProvider()
	}
)

// Start starts the MuxProvider. MuxProvider can only be started once, and once they are started they will run until
// the provided context is cancelled. The MuxProvider will cancel the context itself if it exits due to an unrecoverable
// error or panic. Connection instability is not unrecoverable: the MuxProvider will detect yamux Session exit and open
// a new session.
func (m *MuxProvider) Start() {
	m.startOnce.Do(func() {
		var err error
		go func() {
			defer func() {
				m.reportFatal(fmt.Errorf("mux provider goroutine stopped! Latest error: %w", err))
				m.connProvider.CloseProvider()
			}()
		connect:
			for {
				if m.shutDown.Err() != nil {
					return
				}
				m.logger.Info("mux session watcher starting")

				var conn net.Conn
				conn, err = m.connProvider.GetConnection()
				if err != nil {
					continue connect
				}

				var session *yamux.Session
				session, err = m.sessionFn(conn)
				go observeYamuxSession(session, observerLabels(session.LocalAddr().String(), session.RemoteAddr().String(), "conn", m.name))
				if err != nil {
					m.logger.Fatal("yamux session creation failed", tag.Error(err))
					metrics.MuxErrors.WithLabelValues(m.metricLabels...).Inc()
					continue connect
				}

				m.setNewTransport(session, conn)
				metrics.MuxConnectionEstablish.WithLabelValues(m.metricLabels...).Inc()
				<-session.CloseChan()
				m.onDisconnectFn()
			}
		}()
	})
}
