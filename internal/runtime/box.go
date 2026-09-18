package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/audit"
	"github.com/SofiaFlux/meeseek-collective/internal/capabilities"
	"github.com/SofiaFlux/meeseek-collective/internal/clock"
	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/evidence"
	"github.com/SofiaFlux/meeseek-collective/internal/execution"
	"github.com/SofiaFlux/meeseek-collective/internal/executors"
	"github.com/SofiaFlux/meeseek-collective/internal/memory"
	"github.com/SofiaFlux/meeseek-collective/internal/observability"
	"github.com/SofiaFlux/meeseek-collective/internal/operations"
	"github.com/SofiaFlux/meeseek-collective/internal/policy"
	"github.com/SofiaFlux/meeseek-collective/internal/purpose"
	"github.com/SofiaFlux/meeseek-collective/internal/resources"
	"github.com/SofiaFlux/meeseek-collective/internal/scheduler"
	state "github.com/SofiaFlux/meeseek-collective/internal/state/sqlite"
	"github.com/SofiaFlux/meeseek-collective/internal/teb"
	"github.com/SofiaFlux/meeseek-collective/internal/verification"
	"github.com/SofiaFlux/meeseek-collective/internal/wake"
)

const defaultLeaseDuration = 5 * time.Minute

type Config struct {
	StatePath           string
	EvidencePath        string
	Clock               clock.Clock
	CollectiveID        domain.ID
	PolicyEngine        policy.PolicyEngine
	OperationProviders  []operations.Provider
	CapabilityProviders []capabilities.Provider
	Executors           map[string]executors.Executor
	TEBProfile          teb.Profile
	LeaseDuration       time.Duration
}

type Box struct {
	Store         *state.Store
	Clock         clock.Clock
	Purpose       *purpose.Service
	Execution     *execution.Service
	Evidence      *evidence.Store
	Verification  *verification.Service
	Resources     *resources.Service
	Operations    *operations.Service
	Capabilities  *capabilities.Registry
	Scheduler     *scheduler.Service
	Wake          *wake.Service
	Memory        *memory.Service
	Audit         *audit.Service
	Observability *observability.Bridge
	Policy        policy.PolicyEngine
	Executors     map[string]executors.Executor
	TEBProfile    teb.Profile
	CollectiveID  domain.ID

	shutdown     chan struct{}
	shutdownOnce sync.Once
	closeOnce    sync.Once
	closeErr     error
}

func Open(ctx context.Context, cfg Config) (*Box, error) {
	if ctx == nil {
		return nil, errors.New("runtime context is required")
	}
	cfg.StatePath = strings.TrimSpace(cfg.StatePath)
	cfg.EvidencePath = strings.TrimSpace(cfg.EvidencePath)
	cfg.CollectiveID = domain.ID(strings.TrimSpace(string(cfg.CollectiveID)))
	if cfg.StatePath == "" || cfg.EvidencePath == "" {
		return nil, errors.New("state and evidence paths are required")
	}
	if cfg.CollectiveID == "" {
		return nil, errors.New("collective id is required")
	}
	if cfg.PolicyEngine == nil {
		return nil, errors.New("policy engine is required")
	}
	if cfg.Clock == nil {
		cfg.Clock = clock.System{}
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = defaultLeaseDuration
	}
	if cfg.TEBProfile.Name == "" {
		cfg.TEBProfile = teb.UnenforcedProfile("local-host")
	}
	if err := cfg.TEBProfile.Validate(); err != nil {
		return nil, fmt.Errorf("TEB profile: %w", err)
	}

	store, err := state.Open(ctx, cfg.StatePath)
	if err != nil {
		return nil, err
	}
	closeStore := true
	defer func() {
		if closeStore {
			_ = store.DB().Close()
		}
	}()

	purposes := purpose.New(store, cfg.Clock)
	executionSvc := execution.New(store, cfg.Clock, purposes)
	evidenceStore, err := evidence.New(store, cfg.EvidencePath, cfg.Clock)
	if err != nil {
		return nil, err
	}
	verificationSvc := verification.New(store, cfg.Clock, executionSvc)
	resourceSvc := resources.New(store, cfg.Clock)
	operationsSvc := operations.New(store, cfg.Clock, executionSvc, cfg.PolicyEngine, resourceSvc, cfg.CollectiveID, cfg.OperationProviders...)
	capabilityRegistry := capabilities.NewRegistry(store, cfg.Clock, executionSvc, cfg.CapabilityProviders...)
	schedulerSvc := scheduler.New(store, cfg.Clock, purposes, executionSvc, resourceSvc, cfg.LeaseDuration)
	wakeSvc := wake.New(store, cfg.Clock, schedulerSvc)
	memorySvc := memory.New(store, cfg.Clock)
	auditSvc := audit.New(store, cfg.Clock)

	executorSet := make(map[string]executors.Executor, len(cfg.Executors))
	for name, executor := range cfg.Executors {
		name = strings.TrimSpace(name)
		if name != "" && executor != nil {
			executorSet[name] = executor
		}
	}

	box := &Box{
		Store: store,
		Clock: cfg.Clock,
		Purpose: purposes,
		Execution: executionSvc,
		Evidence: evidenceStore,
		Verification: verificationSvc,
		Resources: resourceSvc,
		Operations: operationsSvc,
		Capabilities: capabilityRegistry,
		Scheduler: schedulerSvc,
		Wake: wakeSvc,
		Memory: memorySvc,
		Audit: auditSvc,
		Observability: observability.New(nil),
		Policy: cfg.PolicyEngine,
		Executors: executorSet,
		TEBProfile: cfg.TEBProfile,
		CollectiveID: cfg.CollectiveID,
		shutdown: make(chan struct{}),
	}
	closeStore = false
	return box, nil
}

func (b *Box) Close() error {
	if b == nil {
		return nil
	}
	b.closeOnce.Do(func() {
		_ = b.RequestShutdown(context.Background())
		if b.Store != nil && b.Store.DB() != nil {
			b.closeErr = b.Store.DB().Close()
		}
	})
	return b.closeErr
}

func (b *Box) RequestShutdown(context.Context) error {
	if b == nil {
		return errors.New("runtime box is not configured")
	}
	b.shutdownOnce.Do(func() { close(b.shutdown) })
	return nil
}

func (b *Box) ShutdownRequested() <-chan struct{} {
	if b == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return b.shutdown
}
