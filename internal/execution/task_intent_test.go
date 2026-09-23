package execution

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskIntentRoundTrips(t *testing.T) {
	svc, _, ctx, missionID := newExecutionService(t)
	req := baseTaskRequest(missionID)
	req.Objective = "  Review the proposed change  "
	req.PayloadJSON = json.RawMessage(`{"repo":"summa42","pr":42}`)
	req.IdempotencyKey = "  case-work-42  "

	created, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.Task(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range []struct {
		name string
		got  string
	}{
		{"created objective", created.Objective},
		{"loaded objective", loaded.Objective},
	} {
		if task.got != "Review the proposed change" {
			t.Errorf("%s = %q", task.name, task.got)
		}
	}
	if string(created.PayloadJSON) != `{"pr":42,"repo":"summa42"}` || string(loaded.PayloadJSON) != string(created.PayloadJSON) {
		t.Fatalf("payload did not round trip: created=%s loaded=%s", created.PayloadJSON, loaded.PayloadJSON)
	}
	if created.IdempotencyKey != "case-work-42" || loaded.IdempotencyKey != created.IdempotencyKey {
		t.Fatalf("key did not round trip: created=%q loaded=%q", created.IdempotencyKey, loaded.IdempotencyKey)
	}

	legacy, err := svc.CreateTask(ctx, baseTaskRequest(missionID))
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Objective != "" || string(legacy.PayloadJSON) != "{}" || legacy.IdempotencyKey != "" {
		t.Fatalf("legacy defaults = %#v", legacy)
	}
	if _, err := svc.Task(ctx, legacy.ID); err != nil {
		t.Fatal(err)
	}

	invalid := baseTaskRequest(missionID)
	invalid.PayloadJSON = json.RawMessage(`{"broken":`)
	if _, err := svc.CreateTask(ctx, invalid); err == nil {
		t.Fatal("invalid payload JSON was accepted")
	}

	precise := baseTaskRequest(missionID)
	precise.PayloadJSON = json.RawMessage(`{"sequence":9007199254740993}`)
	stored, err := svc.CreateTask(ctx, precise)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored.PayloadJSON) != `{"sequence":9007199254740993}` {
		t.Fatalf("payload number lost precision: %s", stored.PayloadJSON)
	}
}

func TestCreateTaskIdempotencyReplaysAndRejectsRequestDrift(t *testing.T) {
	svc, _, ctx, missionID := newExecutionService(t)
	req := baseTaskRequest(missionID)
	req.Objective = "Review change"
	req.PayloadJSON = json.RawMessage(`{"repo":"summa42","pr":42}`)
	req.IdempotencyKey = "work-42"
	first, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	replay := req
	replay.PayloadJSON = json.RawMessage(`{ "pr":42, "repo":"summa42" }`)
	second, err := svc.CreateTask(ctx, replay)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("replay created %s, want %s", second.ID, first.ID)
	}
	var count int
	if err := svc.store.DB().QueryRowContext(ctx, "SELECT count(*) FROM tasks WHERE idempotency_key = ?", req.IdempotencyKey).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("key has %d rows, want 1", count)
	}
	drift := req
	drift.PayloadJSON = json.RawMessage(`{"pr":43,"repo":"summa42"}`)
	if _, err := svc.CreateTask(ctx, drift); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("payload drift error = %v, want conflict", err)
	}
	withoutKey := req
	withoutKey.IdempotencyKey = ""
	a, err := svc.CreateTask(ctx, withoutKey)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.CreateTask(ctx, withoutKey)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Fatal("unkeyed requests returned the same task")
	}
}
