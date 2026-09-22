package fieldfeedback

import (
	"context"
	"errors"
	"sync"
)

type fakeSink struct {
	mu        sync.Mutex
	created   map[string]string
	payloads  []IssuePayload
	createCnt int
	lookupCnt int
	loseAck   bool
}

func newFakeSink() *fakeSink {
	return &fakeSink{created: map[string]string{}}
}

func (s *fakeSink) Create(_ context.Context, payload IssuePayload) (string, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCnt++
	s.payloads = append(s.payloads, payload)
	ref := "issue-" + payload.Marker
	s.created[payload.Marker] = ref
	if s.loseAck {
		return "", 0, errors.New("acknowledgement lost")
	}
	return ref, 0, nil
}

func (s *fakeSink) FindByMarker(_ context.Context, marker string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lookupCnt++
	ref, ok := s.created[marker]
	return ref, ok, nil
}

func (s *fakeSink) counts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createCnt, s.lookupCnt
}

func (s *fakeSink) lastPayload() IssuePayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.payloads) == 0 {
		return IssuePayload{}
	}
	return s.payloads[len(s.payloads)-1]
}
