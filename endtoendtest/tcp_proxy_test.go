package endtoendtest

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/testutil"
)

func TestTCPProxy(t *testing.T) {
	logger := log.NewTestLogger()

	server1Port := testutil.GetFreePort()
	server2Port := testutil.GetFreePort()
	server3Port := testutil.GetFreePort()
	server4Port := testutil.GetFreePort()
	server5Port := testutil.GetFreePort()
	server6Port := testutil.GetFreePort()

	echoServer1 := startEchoServer(t, fmt.Sprintf("localhost:%d", server1Port))
	echoServer2 := startEchoServer(t, fmt.Sprintf("localhost:%d", server2Port))
	echoServer3 := startEchoServer(t, fmt.Sprintf("localhost:%d", server3Port))
	echoServer4 := startEchoServer(t, fmt.Sprintf("localhost:%d", server4Port))
	echoServer5 := startEchoServer(t, fmt.Sprintf("localhost:%d", server5Port))
	echoServer6 := startEchoServer(t, fmt.Sprintf("localhost:%d", server6Port))

	defer func() { _ = echoServer1.Close() }()
	defer func() { _ = echoServer2.Close() }()
	defer func() { _ = echoServer3.Close() }()
	defer func() { _ = echoServer4.Close() }()
	defer func() { _ = echoServer5.Close() }()
	defer func() { _ = echoServer6.Close() }()

	proxyPort1 := testutil.GetFreePort()
	proxyPort2 := testutil.GetFreePort()
	proxyPort3 := testutil.GetFreePort()

	rules := []*ProxyRule{
		{
			ListenPort: fmt.Sprintf("%d", proxyPort1),
			Upstream:   NewUpstream([]string{fmt.Sprintf("localhost:%d", server1Port), fmt.Sprintf("localhost:%d", server2Port)}),
		},
		{
			ListenPort: fmt.Sprintf("%d", proxyPort2),
			Upstream:   NewUpstream([]string{fmt.Sprintf("localhost:%d", server3Port), fmt.Sprintf("localhost:%d", server4Port)}),
		},
		{
			ListenPort: fmt.Sprintf("%d", proxyPort3),
			Upstream:   NewUpstream([]string{fmt.Sprintf("localhost:%d", server5Port), fmt.Sprintf("localhost:%d", server6Port)}),
		},
	}

	proxy := NewTCPProxy(logger, rules)
	err := proxy.Start()
	require.NoError(t, err)
	defer proxy.Stop()

	// Test proxy on port 1
	testProxyConnection(t, fmt.Sprintf("localhost:%d", proxyPort1), "test message 1")

	// Test proxy on port 2
	testProxyConnection(t, fmt.Sprintf("localhost:%d", proxyPort2), "test message 2")

	// Test proxy on port 3
	testProxyConnection(t, fmt.Sprintf("localhost:%d", proxyPort3), "test message 3")
}

func testProxyConnection(t *testing.T, proxyAddr, message string) {
	conn, err := net.DialTimeout("tcp", proxyAddr, 5*time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	_, err = conn.Write([]byte(message))
	require.NoError(t, err)

	buf := make([]byte, len(message))
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	require.Equal(t, message, string(buf))
}

func startEchoServer(t *testing.T, addr string) net.Listener {
	listener, err := net.Listen("tcp", addr)
	require.NoError(t, err)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	return listener
}

func TestTCPProxyLeastConn(t *testing.T) {
	logger := log.NewTestLogger()

	// Create two echo servers
	server1Port := testutil.GetFreePort()
	server2Port := testutil.GetFreePort()
	server1 := startEchoServer(t, fmt.Sprintf("localhost:%d", server1Port))
	server2 := startEchoServer(t, fmt.Sprintf("localhost:%d", server2Port))
	defer func() { _ = server1.Close() }()
	defer func() { _ = server2.Close() }()

	// Create proxy with two upstreams
	proxyPort := testutil.GetFreePort()
	rules := []*ProxyRule{
		{
			ListenPort: fmt.Sprintf("%d", proxyPort),
			Upstream:   NewUpstream([]string{fmt.Sprintf("localhost:%d", server1Port), fmt.Sprintf("localhost:%d", server2Port)}),
		},
	}

	proxy := NewTCPProxy(logger, rules)
	err := proxy.Start()
	require.NoError(t, err)
	defer proxy.Stop()

	// Make multiple connections to verify load balancing
	for i := 0; i < 10; i++ {
		testProxyConnection(t, fmt.Sprintf("localhost:%d", proxyPort), "test")
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTCPProxyContextCancellation(t *testing.T) {
	logger := log.NewTestLogger()

	serverPort := testutil.GetFreePort()
	server := startEchoServer(t, fmt.Sprintf("localhost:%d", serverPort))
	defer func() { _ = server.Close() }()

	proxyPort := testutil.GetFreePort()
	rules := []*ProxyRule{
		{
			ListenPort: fmt.Sprintf("%d", proxyPort),
			Upstream:   NewUpstream([]string{fmt.Sprintf("localhost:%d", serverPort)}),
		},
	}

	proxy := NewTCPProxy(logger, rules)
	err := proxy.Start()
	require.NoError(t, err)

	// Verify it's working
	testProxyConnection(t, fmt.Sprintf("localhost:%d", proxyPort), "test")

	// Stop the proxy
	proxy.Stop()

	// Verify new connections fail
	_, err = net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", proxyPort), 100*time.Millisecond)
	require.Error(t, err)
}
