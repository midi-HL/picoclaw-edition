// Package local implements offline speech-to-text by driving the sherpa-onnx
// command-line tool as an external process.
//
// Design notes:
//   - sherpa-onnx is NOT linked into this binary. It is downloaded at runtime by
//     the user (see the ASR page in the web UI) and invoked via exec. This keeps
//     PicoClaw a pure-Go, CGO-free binary that still cross-compiles for every
//     release target.
//   - The CLI is `sherpa-onnx-offline`, which prints one JSON line per file
//     containing the recognized text.
//   - Only non-streaming is used: PicoClaw always transcribes a complete audio
//     file before handing the text to the model, so streaming buys nothing.
package local

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/audio/asr"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// Transcriber runs sherpa-onnx-offline for each transcription request.
type Transcriber struct {
	binaryPath string
	modelDir   string
	modelType  string // "paraformer" | "sensevoice" | "zipformer-ctc" | "transducer"
	threads    int
	language   string
	timeout    time.Duration
}

// New builds a local transcriber from configuration.
func New(cfg *config.LocalASRConfig) (*Transcriber, error) {
	if cfg == nil {
		return nil, fmt.Errorf("local asr config is nil")
	}
	if strings.TrimSpace(cfg.BinaryPath) == "" {
		return nil, fmt.Errorf("local asr: binary_path is required")
	}
	if _, err := os.Stat(cfg.BinaryPath); err != nil {
		return nil, fmt.Errorf("local asr: binary not found at %s: %w", cfg.BinaryPath, err)
	}
	if strings.TrimSpace(cfg.ModelDir) == "" {
		return nil, fmt.Errorf("local asr: model_dir is required")
	}

	threads := cfg.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
		if threads > 4 {
			threads = 4 // beyond 4 the gain is negligible and CPU spikes
		}
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	return &Transcriber{
		binaryPath: cfg.BinaryPath,
		modelDir:   cfg.ModelDir,
		modelType:  strings.TrimSpace(cfg.ModelType),
		threads:    threads,
		language:   strings.TrimSpace(cfg.Language),
		timeout:    timeout,
	}, nil
}

func (t *Transcriber) Name() string { return "sherpa-onnx-local" }

// Transcribe runs the CLI and returns the recognized text.
func (t *Transcriber) Transcribe(ctx context.Context, audioFilePath string) (*asr.TranscriptionResponse, error) {
	if _, err := os.Stat(audioFilePath); err != nil {
		return nil, fmt.Errorf("local asr: audio file not found: %w", err)
	}

	args, err := t.buildArgs(audioFilePath)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, t.binaryPath, args...)
	cmd.Dir = t.modelDir

	// sherpa-onnx writes model loading info to stderr and the JSON result to
	// stdout. Keep them separate so a noisy stderr cannot corrupt parsing.
	var stdout strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &loggingWriter{}

	start := time.Now()
	if runErr := cmd.Run(); runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("local asr: transcription timed out after %s", t.timeout)
		}
		return nil, fmt.Errorf("local asr: sherpa-onnx failed: %w", runErr)
	}

	text, err := parseJSONText(stdout.String())
	if err != nil {
		return nil, err
	}

	elapsed := time.Since(start)
	logger.InfoCF("voice", "Local ASR transcription complete", map[string]any{
		"elapsed_ms": elapsed.Milliseconds(),
		"text_len":   len([]rune(text)),
		"model_type": t.modelType,
	})

	return &asr.TranscriptionResponse{
		Text:     text,
		Language: t.language,
		Duration: elapsed.Seconds(),
	}, nil
}

// buildArgs maps the configured model type to the right CLI flags.
func (t *Transcriber) buildArgs(wavPath string) ([]string, error) {
	tokens := filepath.Join(t.modelDir, "tokens.txt")
	if _, err := os.Stat(tokens); err != nil {
		return nil, fmt.Errorf("local asr: tokens.txt not found in %s", t.modelDir)
	}

	args := []string{
		"--tokens=" + tokens,
		fmt.Sprintf("--num-threads=%d", t.threads),
		"--decoding-method=greedy_search",
	}

	switch t.modelType {
	case "sensevoice":
		model := firstExisting(t.modelDir, "model.int8.onnx", "model.onnx")
		if model == "" {
			return nil, fmt.Errorf("local asr: sense-voice model not found in %s", t.modelDir)
		}
		args = append(args, "--sense-voice-model="+model)
		if t.language != "" {
			args = append(args, "--sense-voice-language="+t.language)
		}
	case "zipformer-ctc":
		model := firstExisting(t.modelDir, "model.int8.onnx", "model.onnx")
		if model == "" {
			return nil, fmt.Errorf("local asr: zipformer-ctc model not found in %s", t.modelDir)
		}
		args = append(args, "--zipformer-ctc-model="+model)
	case "transducer":
		enc := firstExisting(t.modelDir, "encoder.int8.onnx", "encoder.onnx")
		dec := firstExisting(t.modelDir, "decoder.int8.onnx", "decoder.onnx")
		join := firstExisting(t.modelDir, "joiner.int8.onnx", "joiner.onnx")
		if enc == "" || dec == "" || join == "" {
			return nil, fmt.Errorf("local asr: transducer files missing in %s", t.modelDir)
		}
		args = append(args, "--encoder="+enc, "--decoder="+dec, "--joiner="+join)
	default: // paraformer
		model := firstExisting(t.modelDir, "model.int8.onnx", "model.onnx")
		if model == "" {
			return nil, fmt.Errorf("local asr: paraformer model not found in %s", t.modelDir)
		}
		args = append(args, "--paraformer="+model)
	}

	return append(args, wavPath), nil
}

func firstExisting(dir string, names ...string) string {
	for _, n := range names {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// parseJSONText extracts the "text" field from the JSON line sherpa-onnx prints.
// Older/newer builds may print several lines; we scan for the last valid object
// that has a text field.
func parseJSONText(stdout string) (string, error) {
	var text string
	found := false

	scanner := bufio.NewScanner(strings.NewReader(stdout))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var obj struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		text = obj.Text
		found = true
	}
	if !found {
		return "", fmt.Errorf("local asr: no transcription result in sherpa-onnx output")
	}
	return strings.TrimSpace(text), nil
}

// loggingWriter forwards sherpa-onnx stderr into the debug log instead of
// letting it pollute stdout.
type loggingWriter struct{}

func (w *loggingWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		logger.DebugCF("voice", "sherpa-onnx", map[string]any{"output": msg})
	}
	return len(p), nil
}

func init() {
	// Self-register so asr.DetectTranscriber can construct us without an
	// import cycle.
	asr.RegisterLocalFactory(func(cfg *config.LocalASRConfig) (asr.Transcriber, error) {
		return New(cfg)
	})
}
