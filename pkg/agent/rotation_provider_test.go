package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

// provider_rotation with no multi-key expansion must mark the real entries so
// the chain rotates across providers.
func TestProviderRotation_MarksRealEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.json")
	raw := `{
  "version": 3,
  "agents": { "defaults": { "model_name": "main", "provider": "openai" } },
  "model_list": [
    { "model_name": "main", "provider": "openai", "model": "m1",
      "api_base": "https://a.invalid/v1", "api_keys": ["k1"],
      "provider_rotation": true, "enabled": true },
    { "model_name": "alt", "provider": "openai", "model": "m2",
      "api_base": "https://b.invalid/v1", "api_keys": ["k2"],
      "provider_rotation": true, "enabled": true }
  ]
}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}

	cands := resolveModelCandidates(cfg, "openai", "main", []string{"alt"})
	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(cands))
	}
	for _, c := range cands {
		if !c.Rotate {
			t.Fatalf("provider_rotation should mark %s for rotation", c.Model)
		}
	}
}
