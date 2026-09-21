package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/SofiaFlux/meeseek-collective/internal/control"
	"github.com/SofiaFlux/meeseek-collective/internal/feedbackgithub"
	"github.com/SofiaFlux/meeseek-collective/internal/fieldfeedback"
	"github.com/SofiaFlux/meeseek-collective/internal/domain"
	"github.com/SofiaFlux/meeseek-collective/internal/localconfig"
	"github.com/SofiaFlux/meeseek-collective/internal/policy"
	meeseekruntime "github.com/SofiaFlux/meeseek-collective/internal/runtime"
	state "github.com/SofiaFlux/meeseek-collective/internal/state/sqlite"
)

const controlShutdownTimeout = 5 * time.Second

type controlLifecycle interface {
	Serve(net.Listener) error
	Close(context.Context) error
}

type startupMaterial struct {
	policyEngine   *policy.OPAEngine
	ownerPublicKey ed25519.PublicKey
}

type boxStatusProvider struct {
	box *meeseekruntime.Box
}

func (p boxStatusProvider) Status(ctx context.Context) (control.StatusDTO, error) {
	if p.box == nil || p.box.Store == nil {
		return control.StatusDTO{}, errors.New("Box status provider is not configured")
	}
	var activeTasks, activeAttempts int
	if err := p.box.Store.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM tasks WHERE state NOT IN ('SUCCEEDED', 'FAILED', 'CANCELLED', 'EXPIRED')",
	).Scan(&activeTasks); err != nil {
		return control.StatusDTO{}, err
	}
	if err := p.box.Store.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM attempts WHERE lease_state = 'ACTIVE'",
	).Scan(&activeAttempts); err != nil {
		return control.StatusDTO{}, err
	}
	stateName := "DORMANT"
	if activeTasks != 0 || activeAttempts != 0 {
		stateName = "ACTIVE"
	}
	return control.StatusDTO{
		CollectiveID:   p.box.CollectiveID,
		State:          stateName,
		ActiveTasks:    activeTasks,
		ActiveAttempts: activeAttempts,
	}, nil
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

func run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("Box context is required")
	}
	home, err := localconfig.ResolveHome("")
	if err != nil {
		return err
	}
	cfg, err := localconfig.Load(home)
	if err != nil {
		return fmt.Errorf("load initialized Collective: %w", err)
	}
	material, err := loadStartupMaterial(ctx, cfg)
	if err != nil {
		return err
	}
	feedbackSink, err := buildFeedbackSink(cfg)
	if err != nil {
		return err
	}

	box, err := meeseekruntime.Open(ctx, meeseekruntime.Config{
		StatePath:    cfg.DatabasePath,
		EvidencePath: cfg.EvidencePath,
		CollectiveID: cfg.CollectiveID,
		OwnerPrincipalID: cfg.OwnerPrincipalID,
		FieldFeedback: cfg.FieldFeedback,
		FeedbackSink: feedbackSink,
		PolicyEngine: material.policyEngine,
	})
	if err != nil {
		return fmt.Errorf("open Box runtime: %w", err)
	}
	defer box.Close()

	server, err := control.NewServer(control.ServerConfig{
		AuthToken:        cfg.ControlToken,
		OwnerPrincipalID: cfg.OwnerPrincipalID,
		OwnerPublicKey:   material.ownerPublicKey,
		ChallengeTTL:     2 * time.Minute,
	}, control.Dependencies{
		Status:     boxStatusProvider{box: box},
		Tasks:      box.Execution,
		Approvals:  box.Approvals,
		Feedback:   box.Feedback,
		Sanitizer:  box.Sanitizer,
		FieldObserver: box.FieldObserver,
		Experience: box.Experience,
		Attempts:   box.Execution,
		Operations: box.Operations,
		Shutdown:   box,
	})
	if err != nil {
		return fmt.Errorf("create control server: %w", err)
	}

	endpoint := strings.TrimSpace(os.Getenv("MEESEEK_CONTROL_ENDPOINT"))
	listener, err := control.ListenLocal(endpoint)
	if err != nil {
		return fmt.Errorf("listen on local control endpoint: %w", err)
	}

	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-box.ShutdownRequested():
			cancel()
		case <-serveCtx.Done():
		}
	}()
	return serveControl(serveCtx, listener, server)
}


func buildFeedbackSink(cfg localconfig.Config) (fieldfeedback.Sink, error) {
	if !cfg.FieldFeedback.Enabled || cfg.FieldFeedback.Mode == localconfig.FeedbackModeLocalOnly {
		return nil, nil
	}
	switch cfg.FieldFeedback.Provider {
	case "github":
		tokenFile := strings.TrimSpace(os.Getenv("MEESEEK_FEEDBACK_GITHUB_TOKEN_FILE"))
		if tokenFile == "" {
			return nil, errors.New("GitHub feedback export requires MEESEEK_FEEDBACK_GITHUB_TOKEN_FILE")
		}
		apiBaseURL := strings.TrimSpace(os.Getenv("MEESEEK_FEEDBACK_GITHUB_API_BASE_URL"))
		sink, err := feedbackgithub.NewSink(feedbackgithub.Config{
			APIBaseURL: apiBaseURL,
			Repository: cfg.FieldFeedback.Destination,
			CredentialSource: feedbackgithub.FileCredentialSource{Path: tokenFile},
		})
		if err != nil {
			return nil, fmt.Errorf("configure GitHub feedback sink: %w", err)
		}
		return sink, nil
	default:
		return nil, fmt.Errorf("unsupported field feedback provider %q", cfg.FieldFeedback.Provider)
	}
}

func loadStartupMaterial(ctx context.Context, cfg localconfig.Config) (startupMaterial, error) {
	store, err := state.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return startupMaterial{}, fmt.Errorf("open canonical state for startup: %w", err)
	}
	defer store.DB().Close()

	var (
		policySetID       domain.ID
		moduleName        string
		module            []byte
		policyHash        string
		capabilitiesHash  string
	)
	if err := store.DB().QueryRowContext(ctx,
		"SELECT policy_set_id, module_name, module, policy_hash, capabilities_hash FROM policy_sets WHERE active = 1",
	).Scan(&policySetID, &moduleName, &module, &policyHash, &capabilitiesHash); err != nil {
		return startupMaterial{}, fmt.Errorf("load active policy set: %w", err)
	}
	if policySetID != cfg.ActivePolicySetID {
		return startupMaterial{}, fmt.Errorf("active policy set %s does not match config %s", policySetID, cfg.ActivePolicySetID)
	}
	engine := policy.NewOPAEngine(policy.OPAConfig{
		ModuleName:    moduleName,
		Module:        string(module),
		PolicySetID:   policySetID,
		PolicySetHash: policyHash,
	})
	metadata, err := engine.Metadata()
	if err != nil {
		return startupMaterial{}, fmt.Errorf("validate active policy set: %w", err)
	}
	if metadata.PolicyCapabilitiesHash != capabilitiesHash {
		return startupMaterial{}, errors.New("active policy capability profile does not match the runtime safe profile")
	}

	var publicKey []byte
	if err := store.DB().QueryRowContext(ctx,
		"SELECT public_key FROM principals WHERE principal_id = ? AND principal_kind = 'OWNER'",
		cfg.OwnerPrincipalID,
	).Scan(&publicKey); err != nil {
		return startupMaterial{}, fmt.Errorf("load Owner public key: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return startupMaterial{}, fmt.Errorf("Owner public key has invalid size %d", len(publicKey))
	}
	return startupMaterial{
		policyEngine:   engine,
		ownerPublicKey: append(ed25519.PublicKey(nil), publicKey...),
	}, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
