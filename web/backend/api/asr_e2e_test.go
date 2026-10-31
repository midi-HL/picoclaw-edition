package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
)

// End-to-end: install binary -> download model -> enable -> transcribe.
// Skipped unless PICOCLAW_ASR_E2E=1 (it downloads ~100MB from GitHub).
func TestLocalASR_EndToEnd(t *testing.T) {
	if os.Getenv("PICOCLAW_ASR_E2E") != "1" {
		t.Skip("set PICOCLAW_ASR_E2E=1 to run the real install/download/transcribe test")
	}

	home := t.TempDir()
	t.Setenv("PICOCLAW_HOME", home)

	cfgPath := filepath.Join(home, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"version":3}`), 0o600); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(cfgPath)
	mux := http.NewServeMux()
	h.registerLocalASRRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	post := func(path string) {
		resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader([]byte(`{"model_id":"paraformer-zh-small","enabled":true}`)))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		t.Logf("%s -> %d %s", path, resp.StatusCode, string(b))
	}

	// 1) install binary (wait until the task actually appears, then finish)
	post("/api/asr/local/install")
	if !waitStage(t, srv.URL, "binary") {
		t.Fatal("binary install did not finish")
	}
	if _, err := os.Stat(sherpaBinaryPath()); err != nil {
		t.Fatalf("binary still missing after install: %v", err)
	}

	// 2) download model
	post("/api/asr/local/download-model")
	if !waitStage(t, srv.URL, "model:paraformer-zh-small") {
		t.Fatal("model download did not finish")
	}

	// 3) enable
	post("/api/asr/local/enable")

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Voice.IsLocalASRActive() {
		t.Fatal("local ASR not active after enable")
	}
	t.Logf("active model=%s dir=%s", cfg.Voice.LocalASR.ModelID, cfg.Voice.LocalASR.ModelDir)

	// 4) transcribe the bundled test wav
	wav := filepath.Join(cfg.Voice.LocalASR.ModelDir, "test_wavs", "0.wav")
	f, err := os.Open(wav)
	if err != nil {
		t.Fatalf("test wav missing: %v", err)
	}
	defer f.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("audio", "0.wav")
	io.Copy(part, f)
	mw.Close()

	resp, err := http.Post(srv.URL+"/api/asr/local/test", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	t.Logf("transcribe -> %d %s", resp.StatusCode, string(raw))

	var out struct {
		Text string `json:"text"`
	}
	json.Unmarshal(raw, &out)
	if out.Text == "" {
		t.Fatalf("empty transcription: %s", string(raw))
	}
	fmt.Printf("\n=== TRANSCRIBED: %s ===\n", out.Text)
}

func waitStage(t *testing.T, base, task string) bool {
	deadline := time.Now().Add(15 * time.Minute)
	// Give the goroutine a moment to register the task before polling.
	time.Sleep(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/api/asr/local/progress?task=" + task)
		if err == nil {
			var st map[string]any
			json.NewDecoder(resp.Body).Decode(&st)
			resp.Body.Close()
			stage, _ := st["stage"].(string)
			if stage == "done" {
				return true
			}
			if stage == "error" {
				t.Fatalf("task %s failed: %v", task, st["error"])
			}
			time.Sleep(2 * time.Second)
			continue
		}
		time.Sleep(2 * time.Second)
	}
	return false
}
