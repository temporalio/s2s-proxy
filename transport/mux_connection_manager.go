package transport

import (
	"crypto/tls"
	"fmt"
	"math/rand/v2"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/backoff"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/metrics"
)

type status int32

const (
	statusInitialized status = iota
	statusStarted
	statusStopped
)

const (
	throttleRetryInitialInterval = time.Second
	throttleRetryMaxInterval     = 30 * time.Second
)

var (
	retryPolicy = backoff.NewExponentialRetryPolicy(throttleRetryInitialInterval).
			WithBackoffCoefficient(1.5).
			WithMaximumInterval(throttleRetryMaxInterval)
	// clientMuxDisconnectSleepFn allows setting the behavior of the client when the mux disconnects.
	// We want to avoid a tight retry-loop if the client connects/disconnects many times in a row, but we actually
	// want that behavior in unit tests
	clientMuxDisconnectSleepFn = func() {
		// Don't retry more frequently than once per second.
		// Sleep a random amount between 1s-2s.
		time.Sleep(time.Second + time.Duration(rand.IntN(1000))*time.Millisecond)
	}
)

type (
	muxConnectManager struct {
		config       config.MuxTransportConfig
		muxTransport *muxTransportImpl
		shutdownCh   chan struct{}
		connectedCh  chan struct{} // if closed, means transport is connected.
		mu           sync.Mutex
		wg           sync.WaitGroup
		logger       log.Logger
		status       atomic.Int32
	}
)

func newMuxConnectManager(cfg config.MuxTransportConfig, logger log.Logger) *muxConnectManager {
	cm := &muxConnectManager{
		config: cfg,
		logger: log.With(logger, tag.NewStringTag("Name", cfg.Name), tag.NewStringTag("Mode", string(cfg.Mode))),
	}

	cm.status.Store(int32(statusInitialized))
	return cm
}

func (m *muxConnectManager) open() (MuxTransport, error) {
	if !m.isStarted() {
		return nil, fmt.Errorf("connection manager is not running")
	}

	// Wait for transport to be connected
	// We use a lock here to prevent from reading stale muxTransport data:
	//   t0: (this) open is wait for <-connectedCh
	//   t1: (loop) muxTransport is set and connectedCh is closed
	//   t2: (loop) muxTransport is closed
	//   t3: (loop) muxTransport is set to nil
	//   t4: (this) read muxTransport = nil
	//
	// Holding a lock when reading from <-connectedCh should not cause deadlock.
	// Deadlock can happen if:
	//  1. (this) has the lock
	//  2. (loop) tries to acquire the lock while <-connectedCh is open
	// #2 can't happen because connectedCh is closed before (loop) tries to acquire the lock (see waitForReconnect)

	// TODO: Add some timeout here in case connection can't be fulfilled.
	var muxTransport *muxTransportImpl
	m.mu.Lock()
	select {
	case <-m.shutdownCh:
		m.mu.Unlock()
		return nil, fmt.Errorf("connection manager is not running")

	case <-m.connectedCh:
		muxTransport = m.muxTransport
	}
	m.mu.Unlock()

	//if muxTransport.session.IsClosed() {
	//	return nil, fmt.Errorf("session is closed")
	//}
	return muxTransport, nil
}

func (m *muxConnectManager) isShuttingDown() bool {
	select {
	case <-m.shutdownCh:
		return true
	default:
		return false
	}
}

func (m *muxConnectManager) serverLoop(metricLabels []string, setting config.TCPServerSetting) error {
	var tlsConfig *tls.Config
	var err error
	if tlsCfg := setting.TLS; tlsCfg.IsEnabled() {
		tlsConfig, err = encryption.GetServerTLSConfig(tlsCfg, m.logger)
		if err != nil {
			return err
		}
	}

	listener, err := net.Listen("tcp", setting.ListenAddress)
	if err != nil {
		return err
	}

	m.wg.Add(1)
	go func() {
		defer func() {
			_ = listener.Close()
			m.wg.Done()
		}()

		for {
			select {
			case <-m.shutdownCh:
				return
			default:
				m.logger.Debug("listener accepting new connection")
				// Accept a TCP connection
				server, err := listener.Accept()
				if err != nil {
					if m.isShuttingDown() {
						return
					}

					m.logger.Fatal("listener.Accept failed", tag.Error(err))
					metrics.MuxErrors.WithLabelValues(metricLabels...).Inc()
				}

				var conn net.Conn

				if tlsConfig != nil {
					conn = tls.Server(server, tlsConfig)
				} else {
					conn = server
				}

				m.logger.Info("Accept new connection", tag.NewStringTag("remoteAddr", conn.RemoteAddr().String()))

				m.muxTransport = newMuxTransport(&connAsListener{Conn: conn})
				metrics.MuxConnectionEstablish.WithLabelValues(metricLabels...).Inc()
				m.waitForReconnect()
			}
		}
	}()

	go func() {
		// wait for shutdown
		<-m.shutdownCh
		_ = listener.Close() // this will cause listener.Accept to fail.
	}()

	return nil
}

func (m *muxConnectManager) clientLoop(metricLabels []string, setting config.TCPClientSetting) error {
	var tlsConfig *tls.Config
	var err error
	if tlsCfg := setting.TLS; tlsCfg.IsEnabled() {
		tlsConfig, err = encryption.GetClientTLSConfig(tlsCfg)
		if err != nil {
			return err
		}
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
	connect:
		for {
			select {
			case <-m.shutdownCh:
				return
			default:
				m.logger.Info("mux client dialing server")

				var client net.Conn
				op := func() error {
					var err error
					client, err = net.DialTimeout("tcp", setting.ServerAddress, 5*time.Second)
					return err
				}

				if err := backoff.ThrottleRetry(op, retryPolicy, func(err error) bool {
					m.logger.Error("mux client failed to dial", tag.Error(err))
					return !m.isShuttingDown()
				}); err != nil {
					m.logger.Error("mux client failed to dial with retry", tag.Error(err))
					metrics.MuxErrors.WithLabelValues(metricLabels...).Inc()
					continue connect
				}

				var conn net.Conn
				if tlsConfig != nil {
					conn = tls.Client(client, tlsConfig)
				} else {
					conn = client
				}

				m.muxTransport = newMuxTransport(&connAsListener{Conn: conn})
				metrics.MuxConnectionEstablish.WithLabelValues(metricLabels...).Inc()
				m.waitForReconnect()

				clientMuxDisconnectSleepFn()
			}
		}
	}()

	return nil
}

// observeYamuxSession creates a goroutine that pings the provided yamux session repeatedly and gathers its two
// metrics: Whether the server is alive and how many streams it has open.
func observeYamuxSession(session *yamux.Session, config config.MuxTransportConfig) {
	if session == nil {
		// If we got a null session, we can't even generate tags to report
		return
	}
	labels := []string{session.LocalAddr().String(),
		session.RemoteAddr().String(),
		string(config.Mode),
		config.Name,
	}
	var sessionActive int8 = 1
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for sessionActive == 1 {
		// Prometheus gauges are cheap, but Session.NumStreams() takes a mutex in the session! Only check once per minute
		// to minimize overhead
		select {
		case <-session.CloseChan():
			sessionActive = 0
		case <-ticker.C:
			// wake up so we can report NumStreams
		}
		metrics.MuxSessionOpen.WithLabelValues(labels...).Set(float64(sessionActive))
		if sessionActive == 1 {
			metrics.MuxStreamsActive.WithLabelValues(labels...).Set(float64(session.NumStreams()))
		} else {
			// Clean up the label so we don't report it forever
			metrics.MuxStreamsActive.DeleteLabelValues(labels...)
		}
		metrics.MuxObserverReportCount.WithLabelValues(labels...).Inc()
	}
}

func (m *muxConnectManager) start() error {
	if !m.status.CompareAndSwap(
		int32(statusInitialized),
		int32(statusStarted),
	) {
		return fmt.Errorf("connection manager can't be started. status: %d", m.getStatus())
	}

	m.shutdownCh = make(chan struct{})
	m.connectedCh = make(chan struct{})

	switch m.config.Mode {
	case config.ClientMode:
		m.logger.Info(fmt.Sprintf("Start ConnectMananger with Config: %v", m.config.Client))
		metricLabels := []string{m.config.Client.ServerAddress,
			string(m.config.Mode),
			m.config.Name,
		}
		if err := m.clientLoop(metricLabels, m.config.Client); err != nil {
			return err
		}
	case config.ServerMode:
		m.logger.Info(fmt.Sprintf("Start ConnectMananger with Config: %v", m.config.Server))
		metricLabels := []string{m.config.Server.ListenAddress,
			string(m.config.Mode),
			m.config.Name,
		}
		if err := m.serverLoop(metricLabels, m.config.Server); err != nil {
			return err
		}

	default:
		return fmt.Errorf("invalid multiplexed transport mode: name %s, mode %s", m.config.Name, m.config.Mode)
	}

	return nil
}

func (m *muxConnectManager) isStarted() bool {
	return m.getStatus() == statusStarted
}

func (m *muxConnectManager) getStatus() status {
	return status(m.status.Load())
}

func (m *muxConnectManager) stop() {
	if !m.status.CompareAndSwap(
		int32(statusStarted),
		int32(statusStopped),
	) {
		return
	}

	close(m.shutdownCh)
	m.wg.Wait()
	m.logger.Info("Connection manager stopped")
}

func (m *muxConnectManager) waitForReconnect() {
	// Notify transport is connected
	close(m.connectedCh)

	m.logger.Debug("connected")

	select {
	case <-m.shutdownCh:
		m.muxTransport.closeSession()
	//case <-m.muxTransport.session.CloseChan():
	//	m.muxTransport.closeSession()
	}

	m.logger.Debug("disconnected")

	// Initialize connectedCh and muxTransport. Invariant: connectCh is closed.
	m.mu.Lock()
	m.connectedCh = make(chan struct{})
	muxTransport := m.muxTransport
	m.muxTransport = nil
	m.mu.Unlock()

	// Notify transport is closed after we reset connectedCh to avoid the case that
	// the caller of Open receives an already closed transport.
	close(muxTransport.closeCh)
}
