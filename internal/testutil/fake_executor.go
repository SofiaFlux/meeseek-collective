package testutil

import (
	"context"
	"sync"

	"github.com/SofiaFlux/summa42/internal/executors"
)

type FakeExecutor struct {
	mu     sync.Mutex
	Result executors.ExecutionResult
	Err    error
	Calls  []executors.AttemptEnvelope
}

func (f *FakeExecutor) Start(_ context.Context, envelope executors.AttemptEnvelope) (executors.ExecutionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	envelope.AcceptanceCriteria = append([]string(nil), envelope.AcceptanceCriteria...)
	envelope.VisibleCapabilities = append([]string(nil), envelope.VisibleCapabilities...)
	f.Calls = append(f.Calls, envelope)
	return f.Result, f.Err
}

func (f *FakeExecutor) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Calls)
}

func (f *FakeExecutor) LastCall() (executors.AttemptEnvelope, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.Calls) == 0 {
		return executors.AttemptEnvelope{}, false
	}
	call := f.Calls[len(f.Calls)-1]
	call.AcceptanceCriteria = append([]string(nil), call.AcceptanceCriteria...)
	call.VisibleCapabilities = append([]string(nil), call.VisibleCapabilities...)
	return call, true
}
