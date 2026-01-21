package mux

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
)

// Note: This was an exploratory test demonstrating that a single listener can spawn many mux connections off the same
// port.

type muxServer struct {
	session *yamux.Session
	buf     []byte
	wg      *sync.WaitGroup
}
type muxClientServer struct {
	muxServer *muxServer
	muxClient *yamux.Session
}

func (s *muxServer) Run(t *testing.T) {
	s.wg.Add(1)
	defer s.wg.Done()
	for {
		//t.Log("Listening on yamux", s.session.Addr())
		muxconn, err := s.session.Accept()
		if err != nil {
			//t.Log(err)
			return
		}
		//t.Log("Got a new connection")
		_, err = muxconn.Read(s.buf)
		//t.Log("Read", num, "bytes:", string(s.buf[:num]))
		if err != nil {
			//t.Log(err)
			return
		}
		require.True(t, s.buf[0] == 'H')
	}
}

func TestListenerHandoff(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverConnCh := make(chan net.Conn)
	listenerClosedCh := make(chan struct{})
	go func() {
		serverSide, err := listener.Accept()
		require.NoError(t, err)
		serverConnCh <- serverSide
		err = listener.Close()
		require.NoError(t, err)
		close(listenerClosedCh)
	}()
	clientConn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	serverConn := <-serverConnCh
	<-listenerClosedCh
	_, _ = clientConn.Write([]byte("Hi"))
	buf := make([]byte, 1024)
	n, _ := serverConn.Read(buf)
	require.Equal(t, []byte("Hi"), buf[:n])
	close(serverConnCh)
}

func TestMultiMux(t *testing.T) {
	wg := &sync.WaitGroup{}
	serverCh, shutDownMux, muxListenerAddrCh := make(chan *muxServer), make(chan struct{}), make(chan string)
	go func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Error(err)
			return
		}
		muxListenerAddrCh <- listener.Addr().String()
		for {
			//t.Log("listening on", listener.Addr())
			conn, err := listener.Accept()
			if err != nil {
				//t.Log(err)
				return
			}
			//t.Log("Got a new connection. Making mux")
			mux, err := yamux.Server(conn, nil)
			if err != nil {
				//t.Log(err)
				return
			}

			select {
			case <-shutDownMux:
				_ = mux.Close()
				return
			case serverCh <- &muxServer{mux, make([]byte, 128), wg}:
			}
		}
	}()
	muxListenerAddr := <-muxListenerAddrCh
	muxes := make([]*muxClientServer, 10)
	for i := range 10 {
		conn := dialUntilSuccess(t, muxListenerAddr)
		yamuxClient, err := yamux.Client(conn, nil)
		require.NoError(t, err)
		_, _ = yamuxClient.Ping()
		muxes[i] = &muxClientServer{
			muxServer: <-serverCh,
			muxClient: yamuxClient,
		}
		go muxes[i].muxServer.Run(t)
	}
	time.Sleep(1 * time.Second)
	for i := 0; i < 10; i++ {
		conn, err := muxes[i].muxClient.Open()
		require.NoError(t, err)
		num, err := conn.Write([]byte("Hello, World!"))
		require.NoError(t, err)
		require.Equal(t, num, len("Hello, World!"))
	}
	close(shutDownMux)
	for _, mux := range muxes {
		_ = mux.muxClient.Close()
		_ = mux.muxServer.session.Close()
	}
	wg.Wait()
}

func dialUntilSuccess(t *testing.T, addr string) net.Conn {
	var conn net.Conn
	var err error
	for conn == nil {
		conn, err = net.Dial("tcp", addr)
		if err != nil {
			//t.Log(err)
			time.Sleep(100 * time.Millisecond)
		}
	}
	return conn
}
