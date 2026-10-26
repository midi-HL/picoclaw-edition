package providers

import (
	"context"
	"errors"
	"testing"
)

// collectStartOrder runs ExecuteCandidate n times and records which candidate
// the chain tried first on each request.
func collectStartOrder(t *testing.T, candidates []FallbackCandidate, n int) []string {
	t.Helper()
	chain := NewFallbackChain(NewCooldownTracker(), nil)
	var starts []string
	for i := 0; i < n; i++ {
		_, err := chain.ExecuteCandidate(
			context.Background(),
			candidates,
			func(_ context.Context, c FallbackCandidate) (*LLMResponse, error) {
				// Record the first attempt only, then fail so the chain stops.
				if len(starts) == i {
					starts = append(starts, c.Model)
				}
				return nil, errors.New("stop")
			},
		)
		_ = err
	}
	return starts
}

func TestFallback_RotationDisabled_AlwaysStartsFirst(t *testing.T) {
	candidates := []FallbackCandidate{
		{Provider: "p", Model: "a"},
		{Provider: "p", Model: "b"},
		{Provider: "p", Model: "c"},
	}

	starts := collectStartOrder(t, candidates, 3)
	for i, got := range starts {
		if got != "a" {
			t.Fatalf("request %d started with %q, want a (rotation disabled must keep historical behaviour)", i, got)
		}
	}
}

func TestFallback_RotationEnabled_RoundRobin(t *testing.T) {
	candidates := []FallbackCandidate{
		{Provider: "p", Model: "a", Rotate: true},
		{Provider: "p", Model: "b", Rotate: true},
		{Provider: "p", Model: "c", Rotate: true},
	}

	starts := collectStartOrder(t, candidates, 6)
	want := []string{"a", "b", "c", "a", "b", "c"}
	for i := range want {
		if starts[i] != want[i] {
			t.Fatalf("start order = %v, want %v", starts, want)
		}
	}
}

func TestRotateCandidates_PreservesOrder(t *testing.T) {
	// Only candidates that opted in (Rotate) are rotated.
	in := []FallbackCandidate{
		{Model: "a", Rotate: true},
		{Model: "b", Rotate: true},
		{Model: "c", Rotate: true},
	}

	got := rotateCandidates(in, 1)
	if len(got) != 3 || got[0].Model != "b" || got[1].Model != "c" || got[2].Model != "a" {
		t.Fatalf("rotate(1) = %v", []string{got[0].Model, got[1].Model, got[2].Model})
	}

	// Failover order must stay circular and stable.
	got = rotateCandidates(in, 4) // 4 % 3 == 1
	if got[0].Model != "b" || got[1].Model != "c" || got[2].Model != "a" {
		t.Fatalf("rotate(4) should equal rotate(1)")
	}

	// Zero offset and single item are no-ops.
	if same := rotateCandidates(in, 0); len(same) != 3 || same[0].Model != "a" {
		t.Fatal("rotate(0) must be a no-op")
	}
	single := []FallbackCandidate{{Model: "only", Rotate: true}}
	if got := rotateCandidates(single, 5); len(got) != 1 || got[0].Model != "only" {
		t.Fatal("single candidate must not be rotated")
	}

	// Non-opted-in candidates keep their positions.
	mixed := []FallbackCandidate{
		{Model: "fixed1"},
		{Model: "a", Rotate: true},
		{Model: "b", Rotate: true},
	}
	got = rotateCandidates(mixed, 1)
	if got[0].Model != "fixed1" || got[1].Model != "b" || got[2].Model != "a" {
		t.Fatalf("mixed rotate(1) = %v, want [fixed1 b a]",
			[]string{got[0].Model, got[1].Model, got[2].Model})
	}
}

func TestAnyRotating(t *testing.T) {
	if anyRotating([]FallbackCandidate{{Model: "a"}, {Model: "b"}}) {
		t.Fatal("expected false when no candidate opts in")
	}
	if !anyRotating([]FallbackCandidate{{Model: "a"}, {Model: "b", Rotate: true}}) {
		t.Fatal("expected true when one candidate opts in")
	}
}

// Simulates 3 keys expanded into 3 candidates (all Rotate=true) and verifies
// that consecutive requests start with different keys — the real multi-key
// rotation scenario surfaced through the UI.
func TestRotation_ThreeKeys_RoundRobinKeys(t *testing.T) {
	cands := []FallbackCandidate{
		{Provider: "p", Model: "k1", Rotate: true},
		{Provider: "p", Model: "k2", Rotate: true},
		{Provider: "p", Model: "k3", Rotate: true},
	}

	chain := NewFallbackChain(NewCooldownTracker(), nil)
	var order []string
	for i := 0; i < 6; i++ {
		_, err := chain.ExecuteCandidate(context.Background(), cands,
			func(_ context.Context, c FallbackCandidate) (*LLMResponse, error) {
				order = append(order, c.Model)
				return &LLMResponse{Content: "ok"}, nil
			})
		if err != nil {
			t.Fatal(err)
		}
	}

	want := []string{"k1", "k2", "k3", "k1", "k2", "k3"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("key order = %v, want %v", order, want)
		}
	}
}
