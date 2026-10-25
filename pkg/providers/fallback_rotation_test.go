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
	in := []FallbackCandidate{{Model: "a"}, {Model: "b"}, {Model: "c"}}

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
	single := []FallbackCandidate{{Model: "only"}}
	if got := rotateCandidates(single, 5); len(got) != 1 || got[0].Model != "only" {
		t.Fatal("single candidate must not be rotated")
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
