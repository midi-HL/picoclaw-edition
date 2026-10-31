package api

import (
	"runtime"
	"testing"
)

func TestASRModelCatalogue(t *testing.T) {
	if len(asrModels) != 3 {
		t.Fatalf("expected 3 models, got %d", len(asrModels))
	}
	// Exactly one recommended default, and it is the small paraformer.
	rec := 0
	for _, m := range asrModels {
		if m.Recommended {
			rec++
			if m.ID != "paraformer-zh-small" {
				t.Fatalf("recommended should be paraformer-zh-small, got %s", m.ID)
			}
		}
		if m.ArchiveURL == "" || m.DirName == "" || m.ModelType == "" {
			t.Fatalf("model %s missing required fields", m.ID)
		}
	}
	if rec != 1 {
		t.Fatalf("expected exactly 1 recommended model, got %d", rec)
	}
}

func TestSherpaArchiveForPlatform(t *testing.T) {
	url, prefix, ok := sherpaArchive()
	if !ok {
		t.Skipf("platform %s/%s unsupported", runtime.GOOS, runtime.GOARCH)
	}
	if url == "" || prefix == "" {
		t.Fatal("empty archive url/prefix")
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{0: "?", 512: "512 B", 2048: "2.0 KB", 5 << 20: "5.0 MB"}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Fatalf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
