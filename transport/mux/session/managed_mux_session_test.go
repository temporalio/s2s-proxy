package session

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"

	"github.com/temporalio/s2s-proxy/endtoendtest/proxyassert"
)

func TestManagedSessionLifecycle(t *testing.T) {
	left, right := net.Pipe()
	client, err := yamux.Client(left, nil)
	require.NoError(t, err)
	server, err := yamux.Server(right, nil)
	require.NoError(t, err)
	clientComponentCreated, clientComponentClosed := make(chan struct{}, 1), make(chan struct{}, 1)
	// 0-capacity to test that afterFunc does not block
	clientAfterFuncRan := make(chan struct{})
	managedClient := NewManagedMuxSession(t.Context(), "Hello client!", client, left, []StartManagedComponentFn{
		func(ctx context.Context, id string, session *yamux.Session) {
			clientComponentCreated <- struct{}{}
			go func() {
				<-ctx.Done()
				clientComponentClosed <- struct{}{}
			}()
		},
	}, func() { clientAfterFuncRan <- struct{}{} })
	serverComponentCreated, serverComponentClosed := make(chan struct{}, 1), make(chan struct{}, 1)
	serverAfterFuncRan := make(chan struct{})
	managedServer := NewManagedMuxSession(t.Context(), "Hello server!", server, right, []StartManagedComponentFn{
		func(ctx context.Context, id string, session *yamux.Session) {
			serverComponentCreated <- struct{}{}
			go func() {
				<-ctx.Done()
				serverComponentClosed <- struct{}{}
			}()
		},
	}, func() { serverAfterFuncRan <- struct{}{} })
	// Components should have been created
	proxyassert.RequireCh(t, clientComponentCreated, 20*time.Millisecond, "client component builder should have run")
	proxyassert.RequireCh(t, serverComponentCreated, 20*time.Millisecond, "server component builder should have run")
	// Check we can open and close connections without closing yamux
	conn, err := managedClient.Open()
	require.NoError(t, err)
	err = conn.Close()
	require.NoError(t, err)
	conn, err = managedServer.Open()
	require.NoError(t, err)
	err = conn.Close()
	require.NoError(t, err)
	require.False(t, client.IsClosed())
	require.False(t, server.IsClosed())

	// Close the client. This should hangup the server
	managedClient.Close()
	// Managed channels and components should all be closed
	proxyassert.RequireCh(t, managedClient.CloseChan(), 100*time.Millisecond, "Managed client should be closed")
	proxyassert.RequireCh(t, managedServer.CloseChan(), 100*time.Millisecond, "Managed server should be closed")
	proxyassert.RequireCh(t, clientComponentClosed, 100*time.Millisecond, "Client component should have closed")
	proxyassert.RequireCh(t, serverComponentClosed, 100*time.Millisecond, "Server component should have closed")
	proxyassert.RequireCh(t, clientAfterFuncRan, 100*time.Millisecond, "Client afterfunc should have run")
	proxyassert.RequireCh(t, serverAfterFuncRan, 100*time.Millisecond, "Server afterfunc should have run")
	// Underlying yamux sessions should be closed
	require.True(t, client.IsClosed())
	require.True(t, server.IsClosed())
	// ManagedMuxSessions should report Closed
	require.True(t, managedClient.IsClosed())
	require.True(t, managedServer.IsClosed())
	require.True(t, managedClient.State().State == Closed)
	require.True(t, managedServer.State().State == Closed)
	_, err = managedClient.Open()
	require.Error(t, err)
	_, err = managedServer.Open()
	require.Error(t, err)
}
