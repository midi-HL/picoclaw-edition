package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// TestKeyRotation_RealChain_UsesAllKeys is the end-to-end gate the previous
// round lacked. It drives the real config -> candidate -> provider path against
// a fake OpenAI-compatible server that records the Authorization header, and
// asserts that more than one key is actually used.
//
// Before the resolveAgentFallbacks fix this test fails: multi-key expansion
// wrote ModelConfig.Fallbacks, but resolution read AgentDefaults.ModelFallbacks,
// leaving a single candidate so rotation never engaged.
func TestKeyRotation_RealChain_UsesAllKeys(t *testing.T) {
	var mu sync.Mutex
	var seen []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, strings.TrimSpace(r.Header.Get("Authorization")))
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"model":   "test-model",
			"choices": []any{map[string]any{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "ok"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := `{
  "version": 3,
  "agents": { "defaults": { "model_name": "rotest", "provider": "openai" } },
  "model_list": [{
    "model_name": "rotest",
    "provider": "openai",
    "model": "test-model",
    "api_base": "` + srv.URL + `/v1",
    "api_keys": ["key-aaa", "key-bbb", "key-ccc"],
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

	fallbacks := resolveAgentFallbacks(cfg, nil, &cfg.Agents.Defaults, "rotest")
	cands := resolveModelCandidates(cfg, "openai", "rotest", fallbacks)
	if len(cands) < 3 {
		t.Fatalf("expected >=3 candidates, got %d", len(cands))
	}

	// Build one provider per candidate, exactly like the agent does.
	providersByKey := make(map[string]providers.LLMProvider, len(cands))
	for _, c := range cands {
		mc, err := cfg.GetModelConfig(c.DisplayName)
		if err != nil || mc == nil {
			t.Fatalf("GetModelConfig(%q): %v", c.DisplayName, err)
		}
		p, _, err := providers.CreateProviderFromConfig(mc)
		if err != nil {
			t.Fatalf("CreateProviderFromConfig(%q): %v", c.DisplayName, err)
		}
		providersByKey[c.StableKey()] = p
	}

	chain := providers.NewFallbackChain(providers.NewCooldownTracker(), nil)
	for i := 0; i < 6; i++ {
		_, err := chain.ExecuteCandidate(context.Background(), cands,
			func(ctx context.Context, c providers.FallbackCandidate) (*providers.LLMResponse, error) {
				p := providersByKey[c.StableKey()]
				if p == nil {
					t.Fatalf("no provider for candidate %s", c.StableKey())
				}
				return p.Chat(ctx, []providers.Message{{Role: "user", Content: "hi"}}, nil, c.Model, nil)
			})
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	distinct := map[string]struct{}{}
	for _, a := range seen {
		distinct[a] = struct{}{}
	}
	if len(distinct) < 2 {
		t.Fatalf("expected >=2 distinct API keys to be used, got %d (%v)", len(distinct), seen)
	}
}
