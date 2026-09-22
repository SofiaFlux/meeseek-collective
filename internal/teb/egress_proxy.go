package teb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type EgressPolicy struct {
	AllowedHosts map[string]struct{}
}

func (p EgressPolicy) AllowConnect(hostport string) bool {
	host, port, err := net.SplitHostPort(strings.TrimSpace(hostport))
	if err != nil || host == "" || port == "" {
		return false
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return false
	}
	host = strings.ToLower(host)
	for allowed := range p.AllowedHosts {
		if strings.ToLower(strings.TrimSpace(allowed)) == host {
			return true
		}
	}
	return false
}

type ProxyEndpoint struct {
	Address string
}

type EgressProxy interface {
	Start(ctx context.Context, policy EgressPolicy) (ProxyEndpoint, error)
	Close() error
}

type ConnectProxy struct {
	mu       sync.Mutex
	server   *http.Server
	listener net.Listener
}

func NewConnectProxy() *ConnectProxy {
	return &ConnectProxy{}
}

func (p *ConnectProxy) Start(ctx context.Context, policy EgressPolicy) (ProxyEndpoint, error) {
	if p == nil {
		return ProxyEndpoint{}, errors.New("egress proxy is not configured")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.server != nil {
		return ProxyEndpoint{}, errors.New("egress proxy is already started")
	}
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return ProxyEndpoint{}, err
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleConnect(w, r, policy)
	})}
	p.listener = ln
	p.server = server
	go func() {
		_ = server.Serve(ln)
	}()
	if ctx != nil {
		go func() {
			<-ctx.Done()
			_ = p.Close()
		}()
	}
	return ProxyEndpoint{Address: ln.Addr().String()}, nil
}

func (p *ConnectProxy) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	server := p.server
	listener := p.listener
	p.server = nil
	p.listener = nil
	p.mu.Unlock()
	if server == nil {
		return nil
	}
	err := server.Close()
	if listener != nil {
		_ = listener.Close()
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func handleConnect(w http.ResponseWriter, r *http.Request, policy EgressPolicy) {
	if r.Method != http.MethodConnect {
		http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
		return
	}
	target := strings.TrimSpace(r.Host)
	if !policy.AllowConnect(target) {
		http.Error(w, "target not allowed", http.StatusForbidden)
		return
	}

	upstream, err := (&net.Dialer{}).DialContext(r.Context(), "tcp", target)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
		return
	}
	client, rw, err := hijacker.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	defer client.Close()
	defer upstream.Close()
	if _, err := fmt.Fprint(rw, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if err := rw.Flush(); err != nil {
		return
	}

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, rw.Reader)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		done <- struct{}{}
	}()
	<-done
}
