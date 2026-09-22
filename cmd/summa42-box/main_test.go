package main

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/localconfig"
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

func TestRunRefusesUninitializedHome(t *testing.T) {
	t.Setenv("SUMMA42_HOME", t.TempDir())
	if err := run(t.Context()); err == nil {
		t.Fatal("run succeeded without an initialized Collective")
	}
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



func TestBuildFeedbackSinkDoesNotRequireCredentialWhenDisabled(t *testing.T) {
	t.Setenv("SUMMA42_FEEDBACK_GITHUB_TOKEN_FILE","")
	cfg:=localconfig.Config{FieldFeedback:localconfig.FieldFeedbackConfig{Enabled:false,Mode:localconfig.FeedbackModeLocalOnly}}
	sink,err:=buildFeedbackSink(cfg)
	if err!=nil{t.Fatal(err)}
	if sink!=nil{t.Fatal("disabled feedback unexpectedly created sink")}
}

func TestBuildFeedbackSinkFailsClosedWhenGitHubExportEnabledWithoutCredentialFile(t *testing.T) {
	t.Setenv("SUMMA42_FEEDBACK_GITHUB_TOKEN_FILE","")
	cfg:=localconfig.Config{FieldFeedback:localconfig.FieldFeedbackConfig{
		Enabled:true,Mode:localconfig.FeedbackModeAutoIfAllowed,Provider:"github",Destination:"owner/repo",
	}}
	if _,err:=buildFeedbackSink(cfg);err==nil{t.Fatal("GitHub export started without credential file")}
}
