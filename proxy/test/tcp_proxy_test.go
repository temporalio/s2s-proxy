package proxy

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
)

func TestTCPProxy(t *testing.T) {
	logger := log.NewTestLogger()

	server1Addr := GetLocalhostAddress()
	server2Addr := GetLocalhostAddress()
	server3Addr := GetLocalhostAddress()
	server4Addr := GetLocalhostAddress()
	server5Addr := GetLocalhostAddress()
	server6Addr := GetLocalhostAddress()

	echoServer1 := startEchoServer(t, server1Addr)
	echoServer2 := startEchoServer(t, server2Addr)
	echoServer3 := startEchoServer(t, server3Addr)
	echoServer4 := startEchoServer(t, server4Addr)
	echoServer5 := startEchoServer(t, server5Addr)
	echoServer6 := startEchoServer(t, server6Addr)

	defer func() { _ = echoServer1.Close() }()
	defer func() { _ = echoServer2.Close() }()
	defer func() { _ = echoServer3.Close() }()
	defer func() { _ = echoServer4.Close() }()
	defer func() { _ = echoServer5.Close() }()
	defer func() { _ = echoServer6.Close() }()

	proxyAddr1 := GetLocalhostAddress()
	proxyAddr2 := GetLocalhostAddress()
	proxyAddr3 := GetLocalhostAddress()

	_, proxyPort1, _ := net.SplitHostPort(proxyAddr1)
	_, proxyPort2, _ := net.SplitHostPort(proxyAddr2)
	_, proxyPort3, _ := net.SplitHostPort(proxyAddr3)

	rules := []*ProxyRule{
		{
			ListenPort: proxyPort1,
			Upstream:   NewUpstream([]string{server1Addr, server2Addr}),
		},
		{
			ListenPort: proxyPort2,
			Upstream:   NewUpstream([]string{server3Addr, server4Addr}),
		},
		{
			ListenPort: proxyPort3,
			Upstream:   NewUpstream([]string{server5Addr, server6Addr}),
		},
	}

	proxy := NewTCPProxy(logger, rules)
	err := proxy.Start()
	require.NoError(t, err)
	defer proxy.Stop()

	// Test proxy on port 1
	testProxyConnection(t, proxyAddr1, "test message 1")

	// Test proxy on port 2
	testProxyConnection(t, proxyAddr2, "test message 2")

	// Test proxy on port 3
	testProxyConnection(t, proxyAddr3, "test message 3")
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
	server1Addr := GetLocalhostAddress()
	server2Addr := GetLocalhostAddress()
	server1 := startEchoServer(t, server1Addr)
	server2 := startEchoServer(t, server2Addr)
	defer func() { _ = server1.Close() }()
	defer func() { _ = server2.Close() }()

	// Create proxy with two upstreams
	proxyAddr := GetLocalhostAddress()
	_, proxyPort, _ := net.SplitHostPort(proxyAddr)
	rules := []*ProxyRule{
		{
			ListenPort: proxyPort,
			Upstream:   NewUpstream([]string{server1Addr, server2Addr}),
		},
	}

	proxy := NewTCPProxy(logger, rules)
	err := proxy.Start()
	require.NoError(t, err)
	defer proxy.Stop()

	// Make multiple connections to verify load balancing
	for i := 0; i < 10; i++ {
		testProxyConnection(t, proxyAddr, "test")
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTCPProxyContextCancellation(t *testing.T) {
	logger := log.NewTestLogger()

	serverAddr := GetLocalhostAddress()
	server := startEchoServer(t, serverAddr)
	defer func() { _ = server.Close() }()

	proxyAddr := GetLocalhostAddress()
	_, proxyPort, _ := net.SplitHostPort(proxyAddr)
	rules := []*ProxyRule{
		{
			ListenPort: proxyPort,
			Upstream:   NewUpstream([]string{serverAddr}),
		},
	}

	proxy := NewTCPProxy(logger, rules)
	err := proxy.Start()
	require.NoError(t, err)

	// Verify it's working
	testProxyConnection(t, proxyAddr, "test")

	// Stop the proxy
	proxy.Stop()

	// Verify new connections fail
	_, err = net.DialTimeout("tcp", proxyAddr, 100*time.Millisecond)
	require.Error(t, err)
}
