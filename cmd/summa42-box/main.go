package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/SofiaFlux/summa42/internal/adomcp"
	"github.com/SofiaFlux/summa42/internal/adoreview"
	"github.com/SofiaFlux/summa42/internal/capabilities"
	"github.com/SofiaFlux/summa42/internal/control"
	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/feedbackgithub"
	"github.com/SofiaFlux/summa42/internal/fieldfeedback"
	"github.com/SofiaFlux/summa42/internal/localconfig"
	"github.com/SofiaFlux/summa42/internal/policy"
	summa42runtime "github.com/SofiaFlux/summa42/internal/runtime"
	"github.com/SofiaFlux/summa42/internal/scheduler"
	state "github.com/SofiaFlux/summa42/internal/state/sqlite"
	"github.com/SofiaFlux/summa42/internal/workflow"
	"github.com/SofiaFlux/summa42/internal/workflowcase"
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
	box *summa42runtime.Box
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
	adoProvider, err := buildADOProviderFromEnv()
	if err != nil {
		return err
	}
	var capabilityProviders []capabilities.Provider
	if adoProvider != nil {
		capabilityProviders = append(capabilityProviders, adoProvider)
	}

	box, err := summa42runtime.Open(ctx, summa42runtime.Config{
		StatePath:           cfg.DatabasePath,
		EvidencePath:        cfg.EvidencePath,
		CollectiveID:        cfg.CollectiveID,
		OwnerPrincipalID:    cfg.OwnerPrincipalID,
		FieldFeedback:       cfg.FieldFeedback,
		FeedbackSink:        feedbackSink,
		PolicyEngine:        material.policyEngine,
		CapabilityProviders: capabilityProviders,
	})
	if err != nil {
		return fmt.Errorf("open Box runtime: %w", err)
	}
	defer box.Close()
	if err := assessConfiguredProviders(ctx, box.Capabilities, adoProvider); err != nil {
		return fmt.Errorf("assess configured ADO capability provider: %w", err)
	}

	server, err := control.NewServer(control.ServerConfig{
		AuthToken:        cfg.ControlToken,
		OwnerPrincipalID: cfg.OwnerPrincipalID,
		OwnerPublicKey:   material.ownerPublicKey,
		ChallengeTTL:     2 * time.Minute,
	}, control.Dependencies{
		Status:        boxStatusProvider{box: box},
		Tasks:         box.Execution,
		Missions:      box.Purpose,
		Approvals:     box.Approvals,
		Feedback:      box.Feedback,
		Sanitizer:     box.Sanitizer,
		FieldObserver: box.FieldObserver,
		Experience:    box.Experience,
		Attempts:      box.Execution,
		Operations:    box.Operations,
		Shutdown:      box,
	})
	if err != nil {
		return fmt.Errorf("create control server: %w", err)
	}

	endpoint := strings.TrimSpace(os.Getenv("SUMMA42_CONTROL_ENDPOINT"))
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

func buildADOProviderFromEnv() (*adomcp.Provider, error) {
	command := strings.TrimSpace(os.Getenv("SUMMA42_ADO_MCP_COMMAND"))
	organization := strings.TrimSpace(os.Getenv("SUMMA42_ADO_ORGANIZATION"))
	if command == "" && organization == "" {
		return nil, nil
	}
	if command == "" || organization == "" {
		return nil, errors.New("ADO MCP requires both SUMMA42_ADO_MCP_COMMAND and SUMMA42_ADO_ORGANIZATION")
	}
	return adomcp.New(adomcp.Config{Command: command, Organization: organization})
}

type capabilityAssessor interface {
	AssessProvider(context.Context, string) ([]capabilities.Assessment, error)
}

func assessConfiguredProviders(ctx context.Context, assessor capabilityAssessor, provider *adomcp.Provider) error {
	if provider == nil {
		return nil
	}
	if assessor == nil {
		return errors.New("capability assessor is required")
	}
	_, err := assessor.AssessProvider(ctx, provider.Name())
	return err
}

func buildFeedbackSink(cfg localconfig.Config) (fieldfeedback.Sink, error) {
	if !cfg.FieldFeedback.Enabled || cfg.FieldFeedback.Mode == localconfig.FeedbackModeLocalOnly {
		return nil, nil
	}
	switch cfg.FieldFeedback.Provider {
	case "github":
		tokenFile := strings.TrimSpace(os.Getenv("SUMMA42_FEEDBACK_GITHUB_TOKEN_FILE"))
		if tokenFile == "" {
			return nil, errors.New("GitHub feedback export requires SUMMA42_FEEDBACK_GITHUB_TOKEN_FILE")
		}
		apiBaseURL := strings.TrimSpace(os.Getenv("SUMMA42_FEEDBACK_GITHUB_API_BASE_URL"))
		sink, err := feedbackgithub.NewSink(feedbackgithub.Config{
			APIBaseURL:       apiBaseURL,
			Repository:       cfg.FieldFeedback.Destination,
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
		policySetID      domain.ID
		moduleName       string
		module           []byte
		policyHash       string
		capabilitiesHash string
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
	if len(os.Args) > 1 && os.Args[1] == "run-worker" {
		if err := runWorker(ctx, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "run-observer" {
		if err := runObserver(ctx, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseWorkerFlags(args []string) (pollInterval time.Duration, leaseDuration time.Duration, err error) {
	flags := flag.NewFlagSet("run-worker", flag.ContinueOnError)
	flags.DurationVar(&pollInterval, "poll-interval", 30*time.Second, "interval between scheduler polls")
	flags.DurationVar(&leaseDuration, "lease-duration", 0, "attempt lease duration (0 uses Box default)")
	if err := flags.Parse(args); err != nil {
		return 0, 0, err
	}
	if pollInterval <= 0 {
		return 0, 0, errors.New("run-worker requires a positive --poll-interval")
	}
	if leaseDuration < 0 {
		return 0, 0, errors.New("run-worker requires a non-negative --lease-duration")
	}
	return pollInterval, leaseDuration, nil
}

func splitWorkspaceRootArg(args []string) (workspaceRoot string, rest []string, err error) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--workspace-root" {
			if i+1 >= len(args) {
				return "", nil, errors.New("run-worker requires a value for --workspace-root")
			}
			workspaceRoot = args[i+1]
			i++
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--workspace-root="); ok {
			workspaceRoot = value
			continue
		}
		rest = append(rest, arg)
	}
	return workspaceRoot, rest, nil
}

func workerCapacity(box *summa42runtime.Box) (scheduler.CapacitySnapshot, error) {
	caps := make(map[string]scheduler.CapabilityCapacity, len(box.Executors))
	for kind := range box.Executors {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		caps[kind] = scheduler.CapabilityCapacity{Accessible: true, Enforcement: domain.EnforcementEnforced}
	}
	if len(caps) == 0 {
		return scheduler.CapacitySnapshot{}, errors.New("run-worker has no schedulable capabilities: the Box executor registry is empty, so there is no capability source to advertise")
	}
	return scheduler.CapacitySnapshot{Capabilities: caps}, nil
}

func runWorker(ctx context.Context, args []string) error {
	if ctx == nil {
		return errors.New("Box context is required")
	}
	workspaceRoot, rest, err := splitWorkspaceRootArg(args)
	if err != nil {
		return err
	}
	pollInterval, leaseDuration, err := parseWorkerFlags(rest)
	if err != nil {
		return err
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		return errors.New("run-worker requires --workspace-root")
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
	adoProvider, err := buildADOProviderFromEnv()
	if err != nil {
		return err
	}
	var capabilityProviders []capabilities.Provider
	if adoProvider != nil {
		capabilityProviders = append(capabilityProviders, adoProvider)
	}

	runtimeCfg := summa42runtime.Config{
		StatePath:           cfg.DatabasePath,
		EvidencePath:        cfg.EvidencePath,
		CollectiveID:        cfg.CollectiveID,
		OwnerPrincipalID:    cfg.OwnerPrincipalID,
		FieldFeedback:       cfg.FieldFeedback,
		FeedbackSink:        feedbackSink,
		PolicyEngine:        material.policyEngine,
		CapabilityProviders: capabilityProviders,
	}
	if leaseDuration > 0 {
		runtimeCfg.LeaseDuration = leaseDuration
	}
	box, err := summa42runtime.Open(ctx, runtimeCfg)
	if err != nil {
		return fmt.Errorf("open Box runtime: %w", err)
	}
	defer box.Close()
	if err := assessConfiguredProviders(ctx, box.Capabilities, adoProvider); err != nil {
		return fmt.Errorf("assess configured ADO capability provider: %w", err)
	}
	capacity, err := workerCapacity(box)
	if err != nil {
		return err
	}
	worker, err := scheduler.NewWorker(box.Scheduler, box.Execution, box.Evidence, box.Verification, box.Executors, box.Clock, workspaceRoot)
	if err != nil {
		return err
	}
	return worker.Run(ctx, capacity, pollInterval)
}

type stringSlice []string

func (s *stringSlice) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func parseObserverFlags(args []string) (adoreview.Config, time.Duration, error) {
	var cfg adoreview.Config
	var mission, reviewer, envelope string
	var grantCaps, grantActions, workCaps stringSlice
	var maxSteps int
	var remainingBudget int64
	var pollInterval time.Duration
	flags := flag.NewFlagSet("run-observer", flag.ContinueOnError)
	flags.StringVar(&mission, "mission", "", "mission ID to attach observed review cases to")
	flags.StringVar(&reviewer, "reviewer-id", "", "ADO reviewer identity to observe")
	flags.Var(&grantCaps, "grant-capability", "capability granted to review work (repeatable)")
	flags.Var(&grantActions, "grant-action", "action granted to review work (repeatable)")
	flags.Var(&workCaps, "work-capability", "required work capability (repeatable, defaults to grant capabilities)")
	flags.StringVar(&envelope, "envelope", "", "resource envelope ID for materialized review tasks")
	flags.IntVar(&maxSteps, "max-steps", 0, "maximum steps for observed review cases")
	flags.Int64Var(&remainingBudget, "remaining-budget", 0, "remaining budget for observed review cases")
	flags.StringVar(&cfg.Project, "project", "", "ADO project scope (empty uses org-active scope)")
	flags.StringVar(&cfg.Repository, "repository", "", "ADO repository scope")
	flags.DurationVar(&pollInterval, "poll-interval", 5*time.Minute, "interval between observer polls")
	if err := flags.Parse(args); err != nil {
		return adoreview.Config{}, 0, err
	}
	if strings.TrimSpace(mission) == "" {
		return adoreview.Config{}, 0, errors.New("run-observer requires --mission")
	}
	if strings.TrimSpace(reviewer) == "" {
		return adoreview.Config{}, 0, errors.New("run-observer requires --reviewer-id")
	}
	if len(grantCaps) == 0 {
		return adoreview.Config{}, 0, errors.New("run-observer requires at least one --grant-capability (refusing to observe with an empty grant)")
	}
	if strings.TrimSpace(envelope) == "" {
		return adoreview.Config{}, 0, errors.New("run-observer requires --envelope")
	}
	if maxSteps <= 0 {
		return adoreview.Config{}, 0, errors.New("run-observer requires a positive --max-steps")
	}
	if remainingBudget <= 0 {
		return adoreview.Config{}, 0, errors.New("run-observer requires a positive --remaining-budget")
	}
	if pollInterval <= 0 {
		return adoreview.Config{}, 0, errors.New("run-observer requires a positive --poll-interval")
	}
	if (strings.TrimSpace(cfg.Project) == "") != (strings.TrimSpace(cfg.Repository) == "") {
		return adoreview.Config{}, 0, errors.New("run-observer requires --project and --repository together (optional pair)")
	}
	cfg.MissionID = domain.ID(mission)
	cfg.ReviewerID = reviewer
	cfg.Grant = workflow.Grant{Capabilities: []string(grantCaps), Actions: []string(grantActions)}
	cfg.WorkCapabilities = []string(workCaps)
	cfg.ResourceEnvelopeID = domain.ID(envelope)
	cfg.MaxSteps = maxSteps
	cfg.RemainingBudget = remainingBudget
	return cfg, pollInterval, nil
}

func runObserver(ctx context.Context, args []string) error {
	if ctx == nil {
		return errors.New("Box context is required")
	}
	observerCfg, pollInterval, err := parseObserverFlags(args)
	if err != nil {
		return err
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
	adoProvider, err := buildADOProviderFromEnv()
	if err != nil {
		return err
	}
	if adoProvider == nil {
		return errors.New("run-observer requires ADO MCP configuration (SUMMA42_ADO_MCP_COMMAND and SUMMA42_ADO_ORGANIZATION)")
	}
	var capabilityProviders []capabilities.Provider
	capabilityProviders = append(capabilityProviders, adoProvider)

	box, err := summa42runtime.Open(ctx, summa42runtime.Config{
		StatePath:           cfg.DatabasePath,
		EvidencePath:        cfg.EvidencePath,
		CollectiveID:        cfg.CollectiveID,
		OwnerPrincipalID:    cfg.OwnerPrincipalID,
		FieldFeedback:       cfg.FieldFeedback,
		FeedbackSink:        feedbackSink,
		PolicyEngine:        material.policyEngine,
		CapabilityProviders: capabilityProviders,
	})
	if err != nil {
		return fmt.Errorf("open Box runtime: %w", err)
	}
	defer box.Close()
	if err := assessConfiguredProviders(ctx, box.Capabilities, adoProvider); err != nil {
		return fmt.Errorf("assess configured ADO capability provider: %w", err)
	}
	cases := workflowcase.New(box.Store, box.Clock, box.Purpose)
	return adoreview.Run(ctx, adoProvider, cases, box.Execution, box.Evidence, observerCfg, pollInterval)
}
