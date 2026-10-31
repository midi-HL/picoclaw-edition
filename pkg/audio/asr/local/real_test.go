package local_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sipeed/picoclaw/pkg/audio/asr/local"
	"github.com/sipeed/picoclaw/pkg/config"
)

func TestRealTranscribe(t *testing.T) {
	// Opt-in: needs a real sherpa-onnx install. Set SHERPA_ONNX_TEST_DIR to a
	// directory containing bin/ and the model folder to run it.
	base := os.Getenv("SHERPA_ONNX_TEST_DIR")
	if base == "" {
		t.Skip("set SHERPA_ONNX_TEST_DIR to run the real end-to-end test")
	}
	bin := filepath.Join(base, `bin\sherpa-onnx-v2.0.1-win-x64-shared-MD-Release\bin\sherpa-onnx-offline.exe`)
	modelDir := filepath.Join(base, "sherpa-onnx-paraformer-zh-small-2024-03-09")
	wav := filepath.Join(modelDir, "test_wavs", "0.wav")

	if _, err := os.Stat(bin); err != nil {
		t.Skip("sherpa binary not present")
	}

	tr, err := local.New(&config.LocalASRConfig{
		Enabled:    true,
		BinaryPath: bin,
		ModelDir:   modelDir,
		ModelType:  "paraformer",
		Threads:    2,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res, err := tr.Transcribe(context.Background(), wav)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	t.Logf("TEXT=%q", res.Text)
	if res.Text == "" {
		t.Fatal("empty transcription")
	}
}
