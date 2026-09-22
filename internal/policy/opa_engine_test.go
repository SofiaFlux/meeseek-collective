package policy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/testutil"
)

var testNow = time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)

func testInput() PolicyInput {
	return PolicyInput{
		Risk:           "LOW",
		AuthorityValid: true,
		Now:            testNow,
	}
}

func newTestEngine(module string) *OPAEngine {
	return NewOPAEngine(OPAConfig{
		ModuleName:  "test.rego",
		Module:      module,
		PolicySetID: domain.ID("policy_test"),
		Timeout:     250 * time.Millisecond,
	})
}

func TestOPAForbiddenBuiltinsFailCompilation(t *testing.T) {
	forbidden := map[string]string{
		"http.send":          `http.send({"method": "get", "url": "https://example.invalid"})`,
		"net.lookup_ip_addr": `net.lookup_ip_addr("example.invalid")`,
		"opa.runtime":        `opa.runtime()`,
		"time.now_ns":        `time.now_ns()`,
		"rand.intn":          `rand.intn("seed", 10)`,
	}

	for name, expr := range forbidden {
		t.Run(name, func(t *testing.T) {
			module := fmt.Sprintf(`package summa42

			decision := {"outcome": "ALLOW", "reason_codes": ["safe"]}
			forbidden_probe := %s if { false }
			`, expr)
			_, err := newTestEngine(module).Evaluate(context.Background(), testInput())
			if err == nil {
				t.Fatalf("%s must fail compilation under the restricted profile", name)
			}
		})
	}
}

func TestSafeCapabilitiesExcludeForbiddenBuiltins(t *testing.T) {
	caps := safeCapabilities()
	forbidden := map[string]bool{
		"http.send":          true,
		"net.lookup_ip_addr": true,
		"opa.runtime":        true,
		"time.now_ns":        true,
		"rand.intn":          true,
	}
	for _, builtin := range caps.Builtins {
		if forbidden[builtin.Name] {
			t.Fatalf("forbidden builtin %q present in capability profile", builtin.Name)
		}
	}
	for _, required := range []string{"count", "contains", "json.marshal", "object.get"} {
		found := false
		for _, builtin := range caps.Builtins {
			if builtin.Name == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("required deterministic builtin %q missing", required)
		}
	}
}

func TestOPATimeoutFailsClosed(t *testing.T) {
	module := `package summa42
	decision := {"outcome": "ALLOW", "reason_codes": ["low_risk"]}`
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := newTestEngine(module).Evaluate(ctx, testInput()); err == nil {
		t.Fatal("expired evaluation context must fail closed")
	}
}

func TestOPAStrictBuiltinErrorFailsClosed(t *testing.T) {
	module := `package summa42
	default decision := {"outcome": "DENY", "reason_codes": ["fallback"]}
	decision := {"outcome": "ALLOW", "reason_codes": ["impossible"]} if {
		x := 1 / 0
		x == 0
	}`
	if _, err := newTestEngine(module).Evaluate(context.Background(), testInput()); err == nil {
		t.Fatal("builtin runtime error must fail closed instead of falling back")
	}
}

func TestOPAUndefinedDecisionFailsClosed(t *testing.T) {
	module := `package summa42
	decision := {"outcome": "ALLOW", "reason_codes": ["never"]} if { false }`
	if _, err := newTestEngine(module).Evaluate(context.Background(), testInput()); err == nil {
		t.Fatal("undefined decision must fail closed")
	}
}

func TestOPAAmbiguousDecisionFailsClosed(t *testing.T) {
	module := `package summa42
	decision contains {"outcome": "ALLOW", "reason_codes": ["a"]} if { true }
	decision contains {"outcome": "DENY", "reason_codes": ["b"]} if { true }`
	if _, err := newTestEngine(module).Evaluate(context.Background(), testInput()); err == nil {
		t.Fatal("set-valued/ambiguous decision must fail closed")
	}
}

func TestOPARejectsMalformedOutcome(t *testing.T) {
	module := `package summa42
	decision := {"outcome": "MAYBE", "reason_codes": ["bad"]}`
	if _, err := newTestEngine(module).Evaluate(context.Background(), testInput()); err == nil {
		t.Fatal("unknown policy outcome must fail closed")
	}
}

func TestOPARejectsMismatchedPolicyHash(t *testing.T) {
	module := `package summa42
	decision := {"outcome": "ALLOW", "reason_codes": ["safe"]}`
	engine := NewOPAEngine(OPAConfig{
		ModuleName:    "test.rego",
		Module:        module,
		PolicySetID:   domain.ID("policy_test"),
		PolicySetHash: "not-the-module-hash",
		Timeout:       250 * time.Millisecond,
	})
	if _, err := engine.Evaluate(context.Background(), testInput()); err == nil {
		t.Fatal("configured policy hash that does not match module content must fail closed")
	}
}

func TestOPAValidAllowIncludesDecisionProvenance(t *testing.T) {
	module := `package summa42
	default decision := {"outcome": "DENY", "reason_codes": ["default_deny"]}
	decision := {"outcome": "ALLOW", "reason_codes": ["low_risk"]} if {
		input.risk == "LOW"
		input.authority_valid == true
	}`

	got, err := newTestEngine(module).Evaluate(context.Background(), testInput())
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != domain.PolicyAllow {
		t.Fatalf("outcome = %q, want ALLOW", got.Outcome)
	}
	if got.PolicySetID != domain.ID("policy_test") {
		t.Fatalf("policy set id = %q", got.PolicySetID)
	}
	if got.PolicySetHash == "" || got.PolicyCapabilitiesHash == "" || got.InputDigest == "" {
		t.Fatalf("missing provenance: %#v", got)
	}
	if !got.EvaluatedAt.Equal(testNow) {
		t.Fatalf("evaluated at = %s, want %s", got.EvaluatedAt, testNow)
	}
	if len(got.ReasonCodes) != 1 || got.ReasonCodes[0] != "low_risk" {
		t.Fatalf("reason codes = %#v", got.ReasonCodes)
	}
}

func TestBootstrapMigrationCreatesPolicySets(t *testing.T) {
	store := testutil.OpenStore(t)
	var name string
	if err := store.DB().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='policy_sets'`).Scan(&name); err != nil {
		t.Fatalf("policy_sets table missing: %v", err)
	}
}
