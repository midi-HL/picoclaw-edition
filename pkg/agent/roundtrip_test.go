package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

// Save -> reload must preserve api_keys + key_rotation and still yield
// multiple candidates (guards against virtual entries leaking to disk and
// against api_keys being collapsed to a single key).
func TestMultiKeyRotation_SurvivesSaveReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := `{
  "version": 3,
  "agents": { "defaults": { "model_name": "rotest", "provider": "openai" } },
  "model_list": [{
    "model_name": "rotest", "provider": "openai",
    "model": "sensenova-6.8-flash-lite",
    "api_base": "https://example.invalid/v1",
    "api_keys": ["k1", "k2", "k3"],
    "key_rotation": true, "enabled": true
  }]
}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	reloaded, err := config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}

	// Virtual entries must not be persisted.
	nonVirtual := 0
	for _, m := range reloaded.ModelList {
		if !m.IsVirtual() {
			nonVirtual++
		}
	}
	if nonVirtual != 1 {
		t.Fatalf("expected 1 real model entry after reload, got %d", nonVirtual)
	}

	fallbacks := resolveAgentFallbacks(reloaded, nil, &reloaded.Agents.Defaults, "rotest")
	cands := resolveModelCandidates(reloaded, "openai", "rotest", fallbacks)
	if len(cands) < 3 {
		t.Fatalf("after save/reload expected >=3 candidates, got %d", len(cands))
	}
}
