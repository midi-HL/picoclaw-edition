package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

// TestMultiKeyRotation_EndToEnd_CandidatesFromConfig exercises the REAL
// config -> agent path: LoadConfig (which runs expandMultiKeyModels), then
// resolveAgentFallbacks + resolveModelCandidates.
//
// This is the gate the previous round was missing: fallback_rotation_test.go
// hand-builds candidates and never sees config parsing, so a field mismatch
// between expansion (ModelConfig.Fallbacks) and resolution
// (AgentDefaults.ModelFallbacks) went undetected.
func TestMultiKeyRotation_EndToEnd_CandidatesFromConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := `{
  "version": 3,
  "agents": { "defaults": { "model_name": "rotest", "provider": "openai" } },
  "model_list": [{
    "model_name": "rotest",
    "provider": "openai",
    "model": "sensenova-6.8-flash-lite",
    "api_base": "https://example.invalid/v1",
    "api_keys": ["k1", "k2", "k3"],
    "key_rotation": true,
    "enabled": true
  }]
}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	fallbacks := resolveAgentFallbacks(
		cfg, nil, &cfg.Agents.Defaults, cfg.Agents.Defaults.GetModelName(),
	)
	candidates := resolveModelCandidates(
		cfg,
		cfg.Agents.Defaults.Provider,
		cfg.Agents.Defaults.GetModelName(),
		fallbacks,
	)

	if len(candidates) < 3 {
		t.Fatalf("expected >=3 candidates from 3 api_keys, got %d (fallbacks=%v); "+
			"with a single candidate rotateCandidates early-returns and key_rotation is a no-op",
			len(candidates), fallbacks)
	}

	// Every candidate must carry the rotation flag so the chain actually rotates.
	for _, c := range candidates {
		if !c.Rotate {
			t.Fatalf("candidate %s/%s missing Rotate flag", c.Provider, c.Model)
		}
	}
}

// key_rotation and provider_rotation must be independently controllable:
// a virtual __key_N entry only represents an extra key, so enabling only
// provider_rotation must not rotate keys, and vice versa.
func TestRotation_FlagsAreIndependent(t *testing.T) {
	dir := t.TempDir()

	load := func(t *testing.T, jsonBody string) *config.Config {
		t.Helper()
		p := filepath.Join(dir, "c.json")
		if err := os.WriteFile(p, []byte(jsonBody), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := config.LoadConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}

	const base = `{"version":3,
 "agents":{"defaults":{"model_name":"m","provider":"openai"}},
 "model_list":[{"model_name":"m","provider":"openai","model":"x",
   "api_base":"https://e.invalid/v1","api_keys":["k1","k2"],
   %s "enabled":true}]}`

	// Only provider_rotation: the extra key must NOT rotate.
	cfg := load(t, strings.Replace(base, "%s", `"provider_rotation":true,`, 1))
	fb := resolveAgentFallbacks(cfg, nil, &cfg.Agents.Defaults, "m")
	cands := resolveModelCandidates(cfg, "openai", "m", fb)
	for _, c := range cands {
		if c.Rotate {
			t.Fatalf("provider_rotation alone must not rotate keys; candidate %s has Rotate", c.Model)
		}
	}

	// Only key_rotation: every candidate (primary + virtual keys) rotates.
	cfg = load(t, strings.Replace(base, "%s", `"key_rotation":true,`, 1))
	fb = resolveAgentFallbacks(cfg, nil, &cfg.Agents.Defaults, "m")
	cands = resolveModelCandidates(cfg, "openai", "m", fb)
	if len(cands) < 2 {
		t.Fatalf("expected >=2 candidates, got %d", len(cands))
	}
	for _, c := range cands {
		if !c.Rotate {
			t.Fatalf("key_rotation must set Rotate on %s", c.Model)
		}
	}
}
