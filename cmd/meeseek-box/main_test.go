package main

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

type lifecycleControlServer struct {
	served chan struct{}
	closed chan struct{}
	once   sync.Once
}

func newLifecycleControlServer() *lifecycleControlServer {
	return &lifecycleControlServer{
		served: make(chan struct{}),
		closed: make(chan struct{}),
	}
}

func (s *lifecycleControlServer) Serve(net.Listener) error {
	close(s.served)
	<-s.closed
	return nil
}

func (s *lifecycleControlServer) Close(context.Context) error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

func TestServeControlClosesServerWhenContextIsCancelled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := newLifecycleControlServer()

	done := make(chan error, 1)
	go func() {
		done <- serveControl(ctx, listener, server)
	}()

	select {
	case <-server.served:
	case <-time.After(time.Second):
		t.Fatal("control server did not start serving")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveControl returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serveControl did not stop after context cancellation")
	}

	select {
	case <-server.closed:
	default:
		t.Fatal("control server was not closed")
	}
}
