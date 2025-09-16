package mux

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"
)

type muxServer struct {
	session *yamux.Session
	buf     []byte
}
type muxClientServer struct {
	muxServer *muxServer
	muxClient *yamux.Session
}

func (s *muxServer) Run(t *testing.T) {
	for {
		fmt.Println("Listening on yamux", s.session.Addr())
		muxconn, err := s.session.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println("Got a new connection")
		num, err := muxconn.Read(s.buf)
		fmt.Println("Read", num, "bytes:", string(s.buf[:num]))
		if err != nil {
			fmt.Println(err)
			return
		}
		require.True(t, s.buf[0] == 'H')
	}
}

func TestMultiMux(t *testing.T) {
	serverCh, shutDownMux := make(chan *muxServer), make(chan struct{})
	go func() {
		listener, err := net.Listen("tcp", "127.0.0.1:9001")
		if err != nil {
			t.Error(err)
			return
		}
		for {
			fmt.Println("listening on", listener.Addr())
			conn, err := listener.Accept()
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println("Got a new connection. Making mux")
			mux, err := yamux.Server(conn, nil)
			if err != nil {
				fmt.Println(err)
				return
			}

			select {
			case <-shutDownMux:
				return
			case serverCh <- &muxServer{mux, make([]byte, 128)}:
			}
		}
	}()
	muxes := make([]*muxClientServer, 10)
	for i := range 10 {
		conn := dialForever()
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
}

func dialForever() net.Conn {
	for {
		conn, err := net.Dial("tcp", "127.0.0.1:9001")
		if err != nil {
			fmt.Println(err)
			time.Sleep(100 * time.Millisecond)
		}
		return conn
	}
}
