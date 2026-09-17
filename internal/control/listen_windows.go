//go:build windows

package control

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/Microsoft/go-winio"
)

func DefaultEndpoint() string {
	return `\\.\pipe\meeseek-control`
}

func ListenLocal(pipePath string) (net.Listener, error) {
	if pipePath == "" {
		pipePath = DefaultEndpoint()
	}
	return winio.ListenPipe(pipePath, nil)
}

func localHTTPClient(pipePath string) *http.Client {
	if pipePath == "" {
		pipePath = DefaultEndpoint()
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return winio.DialPipeContext(ctx, pipePath)
		},
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}
}
