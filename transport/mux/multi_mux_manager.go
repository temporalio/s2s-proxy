package mux

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/transport/mux/session"
)

type (
	multiMuxManager struct {
		lifetime            context.Context
		name                string
		muxProvider         MuxProvider // A reference to the MuxProvider that provides muxConnection
		muxIdSequencer      uint64
		muxesLock           sync.RWMutex
		muxes               map[string]session.ManagedMuxSession
		perSessionFactories []session.StartManagedComponentFn
		connectionListeners []OnConnectionListUpdate
		init                sync.Once            // Ensures a MuxManager can only be started once
		hasShutDown         channel.ShutdownOnce // Notify that multiMuxManager has finished shutting down
		logger              log.Logger
	}

	// OnConnectionListUpdate will be called when the list of muxes changes, inside the lock that mutated the list.
	// It is therefore guaranteed that for any update to `muxes`, you can rely on the map passed in to be the latest.
	OnConnectionListUpdate func(muxes map[string]session.ManagedMuxSession)

	// MultiMuxManager is the interface between an asynchronous MuxProvider and some number of readers that need to
	// access established yamux sessions. The underlying MuxProvider will continuously reestablish new yamux sessions
	// up to a configured amount, and a configured list of functions will be called per yamux session creation.
	// Listeners can also be registered to receive the full list of available muxes each time the list of muxes changes.
	MultiMuxManager interface {
		// Start starts the underlying MuxProvider
		Start()
		IsClosed() bool
		CloseChan() <-chan struct{}
		// Address is used by unit tests for dynamic port allocation
		Address() string
		IsUsable() bool
		Describe() string
	}
	MuxProviderBuilder func(AddNewMux, context.Context) (MuxProvider, error)
)

func NewCustomMultiMuxManager(ctx context.Context,
	name string,
	muxProviderBuilder MuxProviderBuilder,
	perSessionFactories []session.StartManagedComponentFn,
	connectionListeners []OnConnectionListUpdate,
	logger log.Logger) (MultiMuxManager, error) {
	muxMgr := &multiMuxManager{
		name:                name,
		muxIdSequencer:      0,
		muxesLock:           sync.RWMutex{},
		muxes:               make(map[string]session.ManagedMuxSession),
		perSessionFactories: perSessionFactories,
		connectionListeners: connectionListeners,
		init:                sync.Once{},
		lifetime:            ctx,
		hasShutDown:         channel.NewShutdownOnce(),
		logger:              log.With(logger, tag.NewStringTag("component", fmt.Sprintf("MuxManager-%s", name))),
	}
	var err error
	muxMgr.muxProvider, err = muxProviderBuilder(muxMgr.AddConnection, muxMgr.lifetime)
	if err != nil {
		return nil, fmt.Errorf("building mux provider failed. %w", err)
	}
	context.AfterFunc(ctx, muxMgr.onClose)
	return muxMgr, nil
}

func (m *multiMuxManager) notifyChange() {
	for _, fn := range m.connectionListeners {
		fn(m.muxes)
	}
}

func (m *multiMuxManager) Address() string {
	return m.muxProvider.Address()
}

func (m *multiMuxManager) IsUsable() bool {
	for _, s := range m.muxes {
		if s.State().State == session.Connected {
			return true
		}
	}
	return false
}

func (m *multiMuxManager) AddConnection(yamuxSession *yamux.Session, conn net.Conn) {
	m.muxesLock.Lock()
	defer m.muxesLock.Unlock()
	if m.lifetime.Err() != nil {
		return
	}
	// ClientConn uses a map of string addresses to connections. So we need to generate unique strings for the map cheaply
	newId := fmt.Sprintf("%d", m.muxIdSequencer)
	m.muxIdSequencer++
	ctx, cancel := context.WithCancel(m.lifetime)
	m.muxes[newId] = session.NewSession(ctx, cancel, newId, yamuxSession, conn, m.perSessionFactories, func() {
		m.unregisterMux(newId)
		// Recycle the transport when it closes
		m.muxProvider.AllowMoreConns(1)
	})
	m.notifyChange()
}

// unregisterMuxIfClosed drops the described mux from the map of active muxes if it's closed. Returns true if it removed a mux
//func (m *multiMuxManager) unregisterMuxIfClosed(id string) bool {
//	m.muxesLock.RLock()
//	mux, exists := m.muxes[id]
//	if !exists {
//		return false
//	}
//	if !mux.IsClosed() {
//		m.muxesLock.RUnlock()
//		return false
//	}
//	m.muxesLock.RUnlock()
//	// Anything could happen here, but IDs are guaranteed not to be re-used and muxes cannot reopen once closed,
//	// so it's always ok to delete this mux at this point.
//	m.unregisterMux(id)
//	return true
//}

// unregisterMux deletes a mux from the map, no questions asked.
func (m *multiMuxManager) unregisterMux(id string) {
	m.muxesLock.Lock()
	delete(m.muxes, id)
	m.notifyChange()
	m.muxesLock.Unlock()
}

func (m *multiMuxManager) onClose() {
	// This Close() blocks until the provider is closed
	m.muxProvider.WaitForClose()
	// Close out all the active muxes. Note, each mux is going to try to remove itself from the map, so keep the mux
	// lock while we're doing this
	m.muxesLock.Lock()
	for _, v := range m.muxes {
		v.Close()
	}
	m.muxesLock.Unlock()
	// Notify shutdown complete
	m.hasShutDown.Shutdown()
}

func (m *multiMuxManager) CloseChan() <-chan struct{} {
	return m.hasShutDown.Channel()
}

func (m *multiMuxManager) IsClosed() bool {
	return m.hasShutDown.IsShutdown()
}

func (m *multiMuxManager) Start() {
	m.init.Do(func() {
		// Start the mux provider
		m.muxProvider.Start()
	})
}
func (m *multiMuxManager) Describe() string {
	m.muxesLock.RLock()
	defer m.muxesLock.RUnlock()
	sb := strings.Builder{}
	sb.WriteString("[MuxManager ")
	sb.WriteString(m.name)
	sb.WriteString(", lifetime.Err=")
	if m.lifetime.Err() != nil {
		sb.WriteString(m.lifetime.Err().Error())
	} else {
		sb.WriteString("nil")
	}
	sb.WriteString(", activeMuxes{")
	for k, v := range m.muxes {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v.Describe())
		sb.WriteString(",")
	}
	sb.WriteString("}")
	sb.WriteString(", cleanedUp=")
	sb.WriteString(strconv.FormatBool(m.hasShutDown.IsShutdown()))
	sb.WriteString("]")
	return sb.String()
}
