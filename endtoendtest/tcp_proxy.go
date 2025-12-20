package endtoendtest

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

type (
	UpstreamServer struct {
		Address string
		conns   atomic.Int64
	}

	Upstream struct {
		Servers []*UpstreamServer
		mu      sync.RWMutex
	}

	ProxyRule struct {
		ListenPort string
		Upstream   *Upstream
	}

	TCPProxy struct {
		rules   []*ProxyRule
		logger  log.Logger
		ctx     context.Context
		cancel  context.CancelFunc
		wg      sync.WaitGroup
		servers []net.Listener
	}
)

func NewUpstream(servers []string) *Upstream {
	upstreamServers := make([]*UpstreamServer, len(servers))
	for i, addr := range servers {
		upstreamServers[i] = &UpstreamServer{Address: addr}
	}
	return &Upstream{Servers: upstreamServers}
}

func (u *Upstream) selectLeastConn() *UpstreamServer {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if len(u.Servers) == 0 {
		return nil
	}

	selected := u.Servers[0]
	minConns := selected.conns.Load()

	for i := 1; i < len(u.Servers); i++ {
		conns := u.Servers[i].conns.Load()
		if conns < minConns {
			minConns = conns
			selected = u.Servers[i]
		}
	}

	return selected
}

func (u *Upstream) incrementConn(server *UpstreamServer) {
	server.conns.Add(1)
}

func (u *Upstream) decrementConn(server *UpstreamServer) {
	server.conns.Add(-1)
}

func NewTCPProxy(logger log.Logger, rules []*ProxyRule) *TCPProxy {
	ctx, cancel := context.WithCancel(context.Background())
	return &TCPProxy{
		rules:  rules,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (p *TCPProxy) Start() error {
	for _, rule := range p.rules {
		listener, err := net.Listen("tcp", ":"+rule.ListenPort)
		if err != nil {
			p.Stop()
			return fmt.Errorf("failed to listen on port %s: %w", rule.ListenPort, err)
		}
		p.servers = append(p.servers, listener)

		p.wg.Add(1)
		go p.handleListener(listener, rule)
	}

	return nil
}

func (p *TCPProxy) Stop() {
	p.cancel()
	for _, server := range p.servers {
		_ = server.Close()
	}
	p.wg.Wait()
}

func (p *TCPProxy) handleListener(listener net.Listener, rule *ProxyRule) {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		clientConn, err := listener.Accept()
		if err != nil {
			select {
			case <-p.ctx.Done():
				return
			default:
				p.logger.Warn("failed to accept connection", tag.Error(err))
				continue
			}
		}

		p.wg.Add(1)
		go p.handleConnection(clientConn, rule)
	}
}

func (p *TCPProxy) handleConnection(clientConn net.Conn, rule *ProxyRule) {
	defer p.wg.Done()
	defer func() { _ = clientConn.Close() }()

	upstream := rule.Upstream.selectLeastConn()
	if upstream == nil {
		p.logger.Error("no upstream servers available")
		return
	}

	rule.Upstream.incrementConn(upstream)
	defer rule.Upstream.decrementConn(upstream)

	serverConn, err := net.DialTimeout("tcp", upstream.Address, 5*time.Second)
	if err != nil {
		p.logger.Warn("failed to connect to upstream", tag.NewStringTag("upstream", upstream.Address), tag.Error(err))
		return
	}
	defer func() { _ = serverConn.Close() }()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(serverConn, clientConn)
		_ = serverConn.Close()
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, serverConn)
		_ = clientConn.Close()
	}()

	wg.Wait()
}
