package memory

import (
	"context"
	"testing"
	"time"

	"github.com/SofiaFlux/summa42/internal/domain"
	"github.com/SofiaFlux/summa42/internal/testutil"
)

func TestClaimSupersessionPreservesTemporalHistory(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))
	svc := New(store, clk)

	oldEvidence := evidenceAt("evidence-old", "hash-old", clk.Now())
	if err := svc.PutEvidence(ctx, EvidenceInput{Ref: oldEvidence, Kind: "DOCUMENT"}); err != nil {
		t.Fatal(err)
	}
	oldClaim, err := svc.AddClaim(ctx, ClaimInput{
		SubjectID:   "meter-1",
		Predicate:   "tariff",
		Statement:   "meter-1 tariff is G11",
		Status:      domain.ClaimSupported,
		Confidence:  0.9,
		ValidFrom:   clk.Now().Add(-time.Hour),
		EvidenceIDs: []domain.ID{oldEvidence.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	beforeReplacement := clk.Now().Add(30 * time.Minute)
	clk.Advance(time.Hour)
	newEvidence := evidenceAt("evidence-new", "hash-new", clk.Now())
	if err := svc.PutEvidence(ctx, EvidenceInput{Ref: newEvidence, Kind: "DOCUMENT"}); err != nil {
		t.Fatal(err)
	}
	newClaim, err := svc.AddClaim(ctx, ClaimInput{
		SubjectID:   "meter-1",
		Predicate:   "tariff",
		Statement:   "meter-1 tariff is G12",
		Status:      domain.ClaimSupported,
		Confidence:  0.95,
		ValidFrom:   clk.Now(),
		EvidenceIDs: []domain.ID{newEvidence.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Supersede(ctx, oldClaim.ID, newClaim.ID, clk.Now()); err != nil {
		t.Fatal(err)
	}

	oldAfter, err := svc.Claim(ctx, oldClaim.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !oldAfter.ValidTo.Equal(clk.Now()) {
		t.Fatalf("old claim valid_to = %s, want %s", oldAfter.ValidTo, clk.Now())
	}

	beliefsBefore, err := svc.BeliefsAt(ctx, beforeReplacement)
	if err != nil {
		t.Fatal(err)
	}
	assertClaimIDs(t, beliefsBefore, oldClaim.ID)

	beliefsAfter, err := svc.BeliefsAt(ctx, clk.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	assertClaimIDs(t, beliefsAfter, newClaim.ID)
}

func TestOverlappingConflictingClaimRecordsContradiction(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 13, 0, 0, 0, time.UTC))
	svc := New(store, clk)

	e1 := evidenceAt("evidence-conflict-a", "hash-a", clk.Now())
	e2 := evidenceAt("evidence-conflict-b", "hash-b", clk.Now())
	if err := svc.PutEvidence(ctx, EvidenceInput{Ref: e1, Kind: "API"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.PutEvidence(ctx, EvidenceInput{Ref: e2, Kind: "API"}); err != nil {
		t.Fatal(err)
	}
	first, err := svc.AddClaim(ctx, ClaimInput{
		SubjectID: "operator-1", Predicate: "active", Statement: "operator-1 is active",
		Status: domain.ClaimSupported, Confidence: 0.8, ValidFrom: clk.Now().Add(-time.Hour), EvidenceIDs: []domain.ID{e1.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.AddClaim(ctx, ClaimInput{
		SubjectID: "operator-1", Predicate: "active", Statement: "operator-1 is inactive",
		Status: domain.ClaimSupported, Confidence: 0.8, ValidFrom: clk.Now().Add(-time.Hour), EvidenceIDs: []domain.ID{e2.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	conflicts, err := svc.Contradictions(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || conflicts[0] != second.ID {
		t.Fatalf("contradictions = %v, want [%s]", conflicts, second.ID)
	}
}

func TestBeliefsAtExcludesEvidenceNotYetAvailable(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 14, 0, 0, 0, time.UTC))
	svc := New(store, clk)

	before := clk.Now().Add(-time.Second)
	evidence := evidenceAt("evidence-later", "hash-later", clk.Now())
	if err := svc.PutEvidence(ctx, EvidenceInput{Ref: evidence, Kind: "WEB"}); err != nil {
		t.Fatal(err)
	}
	claim, err := svc.AddClaim(ctx, ClaimInput{
		SubjectID: "market-1", Predicate: "state", Statement: "market-1 is open",
		Status: domain.ClaimSupported, Confidence: 0.7, ValidFrom: clk.Now().Add(-time.Hour), EvidenceIDs: []domain.ID{evidence.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	beliefs, err := svc.BeliefsAt(ctx, before)
	if err != nil {
		t.Fatal(err)
	}
	if len(beliefs) != 0 {
		t.Fatalf("beliefs before evidence availability = %+v, want none", beliefs)
	}
	beliefs, err = svc.BeliefsAt(ctx, clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	assertClaimIDs(t, beliefs, claim.ID)
}

func TestKnowledgeDeltaDeduplicatesWithoutVerificationByRepetition(t *testing.T) {
	ctx := context.Background()
	store := testutil.OpenStore(t)
	clk := testutil.NewClock(time.Date(2026, 9, 16, 15, 0, 0, 0, time.UTC))
	svc := New(store, clk)

	firstEvidence := evidenceAt("evidence-repeat-a", "hash-repeat-a", clk.Now())
	first, err := svc.Ingest(ctx, KnowledgeDelta{
		ID: "delta-a", SourceID: "agent-a",
		Evidence: []EvidenceInput{{Ref: firstEvidence, Kind: "AGENT_OBSERVATION"}},
		Claims: []ClaimInput{{
			SubjectID: "asset-1", Predicate: "healthy", Statement: "asset-1 is healthy",
			Status: domain.ClaimSupported, Confidence: 0.75, ValidFrom: clk.Now(), EvidenceIDs: []domain.ID{firstEvidence.ID},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("first ingest returned %d claims, want 1", len(first))
	}

	clk.Advance(time.Minute)
	secondEvidence := evidenceAt("evidence-repeat-b", "hash-repeat-b", clk.Now())
	second, err := svc.Ingest(ctx, KnowledgeDelta{
		ID: "delta-b", SourceID: "agent-b",
		Evidence: []EvidenceInput{{Ref: secondEvidence, Kind: "AGENT_OBSERVATION"}},
		Claims: []ClaimInput{{
			SubjectID: "asset-1", Predicate: "healthy", Statement: "asset-1 is healthy",
			Status: domain.ClaimVerified, Confidence: 0.99, ValidFrom: first[0].ValidFrom, EvidenceIDs: []domain.ID{secondEvidence.ID},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].ID != first[0].ID {
		t.Fatalf("repeated claim = %+v, want same canonical claim %s", second, first[0].ID)
	}
	if second[0].Status != domain.ClaimSupported {
		t.Fatalf("repetition upgraded status to %s, want %s", second[0].Status, domain.ClaimSupported)
	}

	matches, err := svc.Search(ctx, "healthy", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].ID != first[0].ID {
		t.Fatalf("FTS matches = %+v, want canonical repeated claim", matches)
	}
}

func evidenceAt(id, hash string, at time.Time) domain.EvidenceRef {
	return domain.EvidenceRef{ID: domain.ID(id), ContentHash: hash, MediaType: "application/json", SizeBytes: 10, CreatedAt: at}
}

func assertClaimIDs(t *testing.T, claims []domain.Claim, want ...domain.ID) {
	t.Helper()
	if len(claims) != len(want) {
		t.Fatalf("claim count = %d, want %d: %+v", len(claims), len(want), claims)
	}
	for i := range want {
		if claims[i].ID != want[i] {
			t.Fatalf("claims[%d].ID = %s, want %s", i, claims[i].ID, want[i])
		}
	}
}
