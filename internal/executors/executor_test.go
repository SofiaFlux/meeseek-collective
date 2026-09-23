package executors_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/SofiaFlux/summa42/internal/executors"
)

func TestAttemptEnvelopeExposesTaskIntent(t *testing.T) {
	envelope := executors.AttemptEnvelope{Objective: "prepare release notes"}
	if envelope.Objective != "prepare release notes" {
		t.Fatalf("objective = %q", envelope.Objective)
	}

	payloadField := reflect.ValueOf(&envelope).Elem().FieldByName("PayloadJSON")
	if !payloadField.IsValid() {
		t.Fatal("AttemptEnvelope has no PayloadJSON field")
	}
	if payloadField.Type() != reflect.TypeOf(json.RawMessage{}) {
		t.Fatalf("PayloadJSON type = %s, want json.RawMessage", payloadField.Type())
	}
	payload := json.RawMessage(`{"release":"v1"}`)
	payloadField.Set(reflect.ValueOf(payload))
	if !reflect.DeepEqual(payloadField.Interface(), payload) {
		t.Fatalf("payload = %s, want %s", payloadField.Bytes(), payload)
	}
}
