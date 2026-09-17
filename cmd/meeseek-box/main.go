package main

import (
	"context"
	"errors"
	"net"
	"os"
	"time"
)

const controlShutdownTimeout = 5 * time.Second

type controlLifecycle interface {
	Serve(net.Listener) error
	Close(context.Context) error
}

func serveControl(ctx context.Context, listener net.Listener, server controlLifecycle) error {
	if ctx == nil {
		return errors.New("control context is required")
	}
	if listener == nil {
		return errors.New("control listener is required")
	}
	if server == nil {
		return errors.New("control server is required")
	}
	defer listener.Close()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), controlShutdownTimeout)
		defer cancel()
		if err := server.Close(shutdownCtx); err != nil {
			return err
		}
		select {
		case err := <-serveErr:
			return err
		case <-shutdownCtx.Done():
			return shutdownCtx.Err()
		}
	}
}

func run(context.Context) error {
	return nil
}

func main() {
	if err := run(context.Background()); err != nil {
		os.Exit(1)
	}
}
