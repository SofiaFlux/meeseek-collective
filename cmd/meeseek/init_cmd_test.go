package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInitCommandCreatesHomeOnce(t *testing.T) {
	home := filepath.Join(t.TempDir(), "collective")
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"init", "--home", home})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		filepath.Join(home, "config.json"),
		filepath.Join(home, "state", "meeseek.db"),
		filepath.Join(home, "keys", "owner.key"),
		filepath.Join(home, "keys", "cube.key"),
		filepath.Join(home, "ceremony", "constitution-root.key"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}

	configBody, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(configBody, &config); err != nil {
		t.Fatal(err)
	}
	if token, _ := config["control_token"].(string); token == "" {
		t.Fatal("config does not contain a generated control_token")
	}

	second := NewRootCommand()
	second.SetArgs([]string{"init", "--home", home})
	second.SetOut(&output)
	second.SetErr(&output)
	if err := second.ExecuteContext(t.Context()); err == nil {
		t.Fatal("second init succeeded; want refusal to replace initialized Collective")
	}
}
