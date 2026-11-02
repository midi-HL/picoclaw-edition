package api

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"os/exec"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/audio/asr/local"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// ---------------------------------------------------------------------------
// Model catalogue
// ---------------------------------------------------------------------------

// asrModelSpec describes one downloadable speech-to-text model.
type asrModelSpec struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ArchiveURL  string `json:"-"`
	DirName     string `json:"-"`
	SizeMB      int    `json:"size_mb"`
	Speed       string `json:"speed"`
	Accuracy    string `json:"accuracy"`
	Memory      string `json:"memory"`
	Languages   string `json:"languages"`
	ModelType   string `json:"model_type"`
	Recommended bool   `json:"recommended"`
	// Custom marks models discovered on disk rather than from the built-in
	// catalogue. They cannot be re-downloaded, only removed.
	Custom bool `json:"custom,omitempty"`
	Description string `json:"description"`
}

const sherpaRelease = "v2.0.1"

var asrModels = []asrModelSpec{
	{
		ID:         "paraformer-zh-small",
		Name:       "paraformer-zh-small",
		ArchiveURL: "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-paraformer-zh-small-2024-03-09.tar.bz2",
		DirName:    "sherpa-onnx-paraformer-zh-small-2024-03-09",
		SizeMB:     50,
		Speed:      "很快（CPU 单线程 RTF 0.1~0.3）",
		Accuracy:   "中文中等偏上，英文一般",
		Memory:     "~150 MB",
		Languages:  "中文、英文",
		ModelType:  "paraformer",
		Recommended: true,
		Description: "体积小、速度快，适合中文为主、日常对话与清晰录音。",
	},
	{
		ID:         "streaming-zipformer-zh-14M",
		Name:       "streaming-zipformer-zh-14M",
		ArchiveURL: "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-streaming-zipformer-zh-14M-2023-02-23.tar.bz2",
		DirName:    "sherpa-onnx-streaming-zipformer-zh-14M-2023-02-23",
		SizeMB:     30,
		Speed:      "极快",
		Accuracy:   "中文中等，英文基本不行",
		Memory:     "~100 MB",
		Languages:  "中文",
		ModelType:  "transducer",
		Description: "最小的中文模型，低配设备首选。",
	},
	{
		ID:         "sensevoice-small",
		Name:       "SenseVoice Small",
		ArchiveURL: "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-2024-07-17.tar.bz2",
		DirName:    "sherpa-onnx-sense-voice-zh-en-ja-ko-yue-2024-07-17",
		SizeMB:     241,
		Speed:      "快",
		Accuracy:   "高",
		Memory:     "~350 MB",
		Languages:  "中文、英文、日文、韩文、粤语",
		ModelType:  "sensevoice",
		Description: "多语言、准确度最高，体积和内存占用也最大。",
	},
}

func asrModelByID(id string) (asrModelSpec, bool) {
	for _, m := range asrModels {
		if m.ID == id {
			return m, true
		}
	}
	return asrModelSpec{}, false
}

// ---------------------------------------------------------------------------
// Progress tracking (in-memory, per task)
// ---------------------------------------------------------------------------

type asrTaskState struct {
	mu        sync.Mutex
	stage     string  // idle|downloading|extracting|installing|done|error
	message   string
	percent   float64
	bytesDone int64
	bytesTotal int64
	err       string
	started   time.Time
}

func (s *asrTaskState) set(stage, message string, percent float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stage = stage
	s.message = message
	s.percent = percent
}

func (s *asrTaskState) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{
		"stage":       s.stage,
		"message":     s.message,
		"percent":     s.percent,
		"bytes_done":  s.bytesDone,
		"bytes_total": s.bytesTotal,
		"error":       s.err,
		"elapsed_ms":  time.Since(s.started).Milliseconds(),
	}
}

// asrTaskManager holds one in-flight download/install at a time.
type asrTaskManager struct {
	mu     sync.Mutex
	tasks  map[string]*asrTaskState
	active string
}

func newASRTaskManager() *asrTaskManager {
	return &asrTaskManager{tasks: make(map[string]*asrTaskState)}
}

func (m *asrTaskManager) begin(key string) (*asrTaskState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != "" {
		if cur, ok := m.tasks[m.active]; ok {
			cur.mu.Lock()
			busy := cur.stage == "downloading" || cur.stage == "extracting" || cur.stage == "installing"
			cur.mu.Unlock()
			if busy {
				return nil, false
			}
		}
	}
	st := &asrTaskState{stage: "downloading", message: "准备中…", started: time.Now()}
	m.tasks[key] = st
	m.active = key
	return st, true
}

func (m *asrTaskManager) get(key string) (*asrTaskState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.tasks[key]
	return st, ok
}

func (m *asrTaskManager) finish(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == key {
		m.active = ""
	}
}

// ---------------------------------------------------------------------------
// Routes
// ---------------------------------------------------------------------------

func (h *Handler) registerLocalASRRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/asr/local/status", h.handleLocalASRStatus)
	mux.HandleFunc("/api/asr/local/models", h.handleLocalASRModels)
	mux.HandleFunc("/api/asr/local/install", h.handleLocalASRInstall)
	mux.HandleFunc("/api/asr/local/download-model", h.handleLocalASRDownloadModel)
	mux.HandleFunc("/api/asr/local/progress", h.handleLocalASRProgress)
	mux.HandleFunc("/api/asr/local/enable", h.handleLocalASREnable)
	mux.HandleFunc("/api/asr/local/test", h.handleLocalASRTest)
	mux.HandleFunc("/api/asr/local/uninstall", h.handleLocalASRUninstall)
	mux.HandleFunc("/api/asr/local/download-custom", h.handleLocalASRDownloadCustom)
}

// GlobalASRDirEnv lets the user relocate the shared ASR directory.
const GlobalASRDirEnv = "PICOCLAW_ASR_DIR"

// GlobalASRDir returns the machine-wide ASR directory shared by every PicoClaw
// instance on this host.
//
// Why shared: sherpa-onnx plus a model is 150-250 MB. Users running several
// instances (different configs / ports) should install it once, not N times.
//
// Resolution order:
//  1. $PICOCLAW_ASR_DIR (explicit override)
//  2. ~/.picoclaw-asr   (default; sits next to ~/.picoclaw)
func GlobalASRDir() string {
	if dir := strings.TrimSpace(os.Getenv(GlobalASRDirEnv)); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		// Last resort: keep it beside the config so nothing breaks.
		return filepath.Join(config.GetHome(), "asr")
	}
	return filepath.Join(home, ".picoclaw-asr")
}

// asrRoot returns the directory this instance reads/writes for ASR data.
//
// It prefers the global shared directory so a single install serves every
// instance. The per-config directory (<config-dir>/asr) is still honoured as a
// fallback when it already contains an install — that keeps existing
// deployments working without forcing a migration.
func (h *Handler) asrRoot() string {
	global := GlobalASRDir()
	if _, err := os.Stat(filepath.Join(global, "bin")); err == nil {
		return global
	}
	// Legacy per-instance location, only if it already has content.
	dir := filepath.Dir(h.configPath)
	if dir != "" && dir != "." {
		legacy := filepath.Join(dir, "asr")
		if _, err := os.Stat(filepath.Join(legacy, "bin")); err == nil {
			return legacy
		}
	}
	return global
}

func (h *Handler) asrBinDir() string    { return filepath.Join(h.asrRoot(), "bin") }
func (h *Handler) asrModelsDir() string { return filepath.Join(h.asrRoot(), "models") }

func sherpaBinaryName() string {
	if runtime.GOOS == "windows" {
		return "sherpa-onnx-offline.exe"
	}
	return "sherpa-onnx-offline"
}

func (h *Handler) sherpaBinaryPath() string {
	return filepath.Join(h.asrBinDir(), sherpaBinaryName())
}

// sherpaArchive returns the release asset for the current platform.
func sherpaArchive() (url, dirPrefix string, ok bool) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		return fmt.Sprintf("https://github.com/k2-fsa/sherpa-onnx/releases/download/%s/sherpa-onnx-%s-win-x64-shared-MD-Release.tar.bz2", sherpaRelease, sherpaRelease),
			fmt.Sprintf("sherpa-onnx-%s-win-x64-shared-MD-Release", sherpaRelease), true
	case "linux/amd64":
		return fmt.Sprintf("https://github.com/k2-fsa/sherpa-onnx/releases/download/%s/sherpa-onnx-%s-linux-x64-shared.tar.bz2", sherpaRelease, sherpaRelease),
			fmt.Sprintf("sherpa-onnx-%s-linux-x64-shared", sherpaRelease), true
	case "linux/arm64":
		// NOTE: the asset is "-shared-cpu" (not "-shared"); the latter 404s.
		return fmt.Sprintf("https://github.com/k2-fsa/sherpa-onnx/releases/download/%s/sherpa-onnx-%s-linux-aarch64-shared-cpu.tar.bz2", sherpaRelease, sherpaRelease),
			fmt.Sprintf("sherpa-onnx-%s-linux-aarch64-shared-cpu", sherpaRelease), true
	}
	return "", "", false
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (h *Handler) handleLocalASRStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg, _ := config.LoadConfig(h.configPath)
	installed := false
	if _, err := os.Stat(h.sherpaBinaryPath()); err == nil {
		installed = true
	}

	downloaded := []string{}
	if entries, err := os.ReadDir(h.asrModelsDir()); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				downloaded = append(downloaded, e.Name())
			}
		}
	}
	sort.Strings(downloaded)

	var voice *config.VoiceConfig
	if cfg != nil {
		voice = &cfg.Voice
	}

	resp := map[string]any{
		"platform_supported": true,
		"installed":          installed,
		"binary_path":        h.sherpaBinaryPath(),
		"models_dir":         h.asrModelsDir(),
		"downloaded":         downloaded,
		// The install is machine-wide; surface where it lives and who else uses it.
		"global_dir":    GlobalASRDir(),
		"is_global":     h.asrRoot() == GlobalASRDir(),
		"other_instances": h.liveInstances(),
	}
	if _, _, ok := sherpaArchive(); !ok {
		resp["platform_supported"] = false
		resp["platform"] = runtime.GOOS + "/" + runtime.GOARCH
	}
	if voice != nil && voice.LocalASR != nil {
		resp["config"] = voice.LocalASR
		resp["asr_source"] = voice.ASRSource
		resp["active"] = voice.IsLocalASRActive()
	}
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleLocalASRModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	type item struct {
		asrModelSpec
		Downloaded bool `json:"downloaded"`
		Active     bool `json:"active"`
	}

	cfg, _ := config.LoadConfig(h.configPath)
	activeID := ""
	if cfg != nil && cfg.Voice.LocalASR != nil {
		activeID = cfg.Voice.LocalASR.ModelID
	}

	out := make([]item, 0, len(asrModels))
	known := make(map[string]bool, len(asrModels))
	for _, m := range asrModels {
		known[m.DirName] = true
		dir := filepath.Join(h.asrModelsDir(), m.DirName)
		_, err := os.Stat(dir)
		out = append(out, item{
			asrModelSpec: m,
			Downloaded:   err == nil,
			Active:       m.ID == activeID,
		})
	}

	// Discover models the user dropped into the models directory by hand.
	// Type is inferred from the files present so the CLI gets the right flags.
	for _, extra := range discoverLocalModels(h.asrModelsDir()) {
		if known[extra.DirName] {
			continue
		}
		out = append(out, item{
			asrModelSpec: extra,
			Downloaded:   true,
			Active:       extra.ID == activeID,
		})
	}

	json.NewEncoder(w).Encode(map[string]any{"models": out})
}

// discoverLocalModels scans the models directory for user-supplied models and
// builds a spec for each, inferring the model type from the files present.
func discoverLocalModels(root string) []asrModelSpec {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var found []asrModelSpec
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "tokens.txt")); err != nil {
			continue // not a usable model directory
		}
		mt := inferModelType(dir)
		if mt == "" {
			continue
		}
		found = append(found, asrModelSpec{
			ID:          e.Name(),
			Name:        e.Name(),
			DirName:     e.Name(),
			ModelType:   mt,
			SizeMB:      dirSizeMB(dir),
			Speed:       "自定义",
			Accuracy:    "自定义",
			Memory:      "未知",
			Languages:   "取决于模型",
			Description: "手动放入 models 目录的模型。",
			Custom:      true,
		})
	}
	return found
}

// inferModelType guesses the CLI flag family from the files in dir.
func inferModelType(dir string) string {
	has := func(names ...string) bool {
		for _, n := range names {
			if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
				return true
			}
		}
		return false
	}
	switch {
	case has("encoder.onnx", "encoder.int8.onnx") && has("joiner.onnx", "joiner.int8.onnx"):
		return "transducer"
	case has("model.int8.onnx", "model.onnx"):
		// SenseVoice directories ship a tokenizer/model pair with this marker.
		if has("tokens.txt") {
			lower := strings.ToLower(dir)
			if strings.Contains(lower, "sense") {
				return "sensevoice"
			}
			if strings.Contains(lower, "zipformer") && strings.Contains(lower, "ctc") {
				return "zipformer-ctc"
			}
		}
		return "paraformer"
	}
	return ""
}

func dirSizeMB(dir string) int {
	var total int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return int(total / (1024 * 1024))
}

func (h *Handler) handleLocalASRInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	url, prefix, ok := sherpaArchive()
	if !ok {
		http.Error(w, fmt.Sprintf("no sherpa-onnx build for %s/%s", runtime.GOOS, runtime.GOARCH), http.StatusBadRequest)
		return
	}

	tasks := h.asrTasks()
	st, started := tasks.begin("binary")
	if !started {
		http.Error(w, "another install is already running", http.StatusConflict)
		return
	}

	go func() {
		defer tasks.finish("binary")
		if err := h.installSherpaBinary(url, prefix, st); err != nil {
			st.mu.Lock()
			st.stage = "error"
			st.err = err.Error()
			st.message = "安装失败"
			st.mu.Unlock()
			logger.ErrorCF("voice", "sherpa-onnx install failed", map[string]any{"error": err.Error()})
			return
		}
		st.set("done", "安装完成", 100)
	}()

	json.NewEncoder(w).Encode(map[string]any{"started": true, "task": "binary"})
}

func (h *Handler) handleLocalASRDownloadModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ModelID string `json:"model_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	spec, ok := asrModelByID(req.ModelID)
	if !ok {
		http.Error(w, "unknown model id", http.StatusBadRequest)
		return
	}

	tasks := h.asrTasks()
	key := "model:" + spec.ID
	st, started := tasks.begin(key)
	if !started {
		http.Error(w, "another install is already running", http.StatusConflict)
		return
	}

	go func() {
		defer tasks.finish(key)
		if err := h.installModel(spec, st); err != nil {
			st.mu.Lock()
			st.stage = "error"
			st.err = err.Error()
			st.message = "下载失败"
			st.mu.Unlock()
			logger.ErrorCF("voice", "model download failed", map[string]any{
				"model": spec.ID, "error": err.Error(),
			})
			return
		}
		st.set("done", "下载完成", 100)
	}()

	json.NewEncoder(w).Encode(map[string]any{"started": true, "task": key})
}

// handleLocalASRDownloadCustom downloads a user-supplied model archive.
//
// Body: {"url": "...", "name": "my-model", "model_type": "paraformer"}
// The archive is extracted into models/<name>. model_type may be omitted, in
// which case it is inferred from the extracted files.
func (h *Handler) handleLocalASRDownloadCustom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL       string `json:"url"`
		Name      string `json:"name"`
		ModelType string `json:"model_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	rawURL := strings.TrimSpace(req.URL)
	if rawURL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(rawURL, "https://") && !strings.HasPrefix(rawURL, "http://") {
		http.Error(w, "url must be http(s)", http.StatusBadRequest)
		return
	}

	name := sanitizeModelDirName(req.Name)
	if name == "" {
		// Derive from the URL's last path segment.
		base := filepath.Base(strings.Split(rawURL, "?")[0])
		base = strings.TrimSuffix(base, filepath.Ext(base))
		name = sanitizeModelDirName(base)
	}
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	target := filepath.Join(h.asrModelsDir(), name)
	if _, err := os.Stat(target); err == nil {
		http.Error(w, "a model with that name already exists", http.StatusConflict)
		return
	}

	tasks := h.asrTasks()
	key := "custom:" + name
	st, started := tasks.begin(key)
	if !started {
		http.Error(w, "another install is already running", http.StatusConflict)
		return
	}

	go func() {
		defer tasks.finish(key)
		if err := h.installCustomModel(rawURL, name, req.ModelType, st); err != nil {
			st.mu.Lock()
			st.stage = "error"
			st.err = err.Error()
			st.message = "下载失败"
			st.mu.Unlock()
			logger.ErrorCF("voice", "custom model download failed", map[string]any{
				"url": rawURL, "error": err.Error(),
			})
			return
		}
		st.set("done", "下载完成", 100)
	}()

	json.NewEncoder(w).Encode(map[string]any{"started": true, "task": key, "name": name})
}

// probeAudioDuration returns the media duration in seconds, or 0 when it cannot
// be determined (ffprobe missing, unsupported container, …). Used only for the
// real-time factor display; transcription works without it.
func probeAudioDuration(path string) float64 {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, ffprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err != nil {
		return 0
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || d <= 0 {
		return 0
	}
	return math.Round(d*1000) / 1000
}

// installCustomModel downloads url and extracts it into models/<name>.
func (h *Handler) installCustomModel(url, name, modelType string, st *asrTaskState) error {
	if err := os.MkdirAll(h.asrModelsDir(), 0o755); err != nil {
		return err
	}
	target := filepath.Join(h.asrModelsDir(), name)

	tmpDir, err := os.MkdirTemp("", "picoclaw-custom-model")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, "model.download")
	if err := h.downloadWithProgress(url, archivePath, st); err != nil {
		return err
	}

	st.set("extracting", "解压中…", 0)
	if err := extractAnyArchive(archivePath, tmpDir, target); err != nil {
		_ = os.RemoveAll(target)
		return err
	}

	// Some archives nest the model in a single top-level directory; flatten it.
	if err := flattenSingleDir(target); err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(target, "tokens.txt")); err != nil {
		return fmt.Errorf("archive does not look like a sherpa-onnx model (missing tokens.txt)")
	}

	mt := strings.TrimSpace(modelType)
	if mt == "" {
		mt = inferModelType(target)
	}
	if mt == "" {
		return fmt.Errorf("could not infer model type; specify model_type explicitly")
	}
	logger.InfoCF("voice", "Custom model installed", map[string]any{
		"name": name, "type": mt,
	})
	return nil
}

// sanitizeModelDirName keeps a safe directory name.
func sanitizeModelDirName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "..", "")
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, name)
	return strings.Trim(name, "-.")
}

// flattenSingleDir moves the contents of a lone subdirectory up one level.
func flattenSingleDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return err
	}
	inner := filepath.Join(dir, entries[0].Name())
	children, err := os.ReadDir(inner)
	if err != nil {
		return err
	}
	for _, c := range children {
		if err := os.Rename(filepath.Join(inner, c.Name()), filepath.Join(dir, c.Name())); err != nil {
			return err
		}
	}
	return os.Remove(inner)
}

func (h *Handler) handleLocalASRProgress(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.URL.Query().Get("task"))
	if key == "" {
		key = "binary"
	}
	st, ok := h.asrTasks().get(key)
	if !ok {
		json.NewEncoder(w).Encode(map[string]any{"stage": "idle"})
		return
	}
	json.NewEncoder(w).Encode(st.snapshot())
}

func (h *Handler) handleLocalASREnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ModelID string `json:"model_id"`
		Threads int    `json:"threads"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, "load config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if !req.Enabled {
		if cfg.Voice.LocalASR != nil {
			cfg.Voice.LocalASR.Enabled = false
		}
		if strings.EqualFold(cfg.Voice.ASRSource, "local") {
			cfg.Voice.ASRSource = "cloud"
		}
		if err := config.SaveConfig(h.configPath, cfg); err != nil {
			http.Error(w, "save config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "enabled": false})
		return
	}

	spec, ok := asrModelByID(req.ModelID)
	if !ok {
		http.Error(w, "unknown model id", http.StatusBadRequest)
		return
	}
	modelDir := filepath.Join(h.asrModelsDir(), spec.DirName)
	if _, err := os.Stat(modelDir); err != nil {
		http.Error(w, "model not downloaded yet", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(h.sherpaBinaryPath()); err != nil {
		http.Error(w, "sherpa-onnx not installed yet", http.StatusBadRequest)
		return
	}

	cfg.Voice.ASRSource = "local"
	cfg.Voice.LocalASR = &config.LocalASRConfig{
		Enabled:    true,
		BinaryPath: h.sherpaBinaryPath(),
		ModelDir:   modelDir,
		ModelID:    spec.ID,
		ModelType:  spec.ModelType,
		Threads:    req.Threads,
	}

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, "save config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Record this instance as a user of the shared install so an uninstall from
	// another instance can warn before deleting.
	h.registerInstance()
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "enabled": true, "model_id": spec.ID})
}

func (h *Handler) handleLocalASRTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Multipart upload: field "audio".
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, "audio file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmpDir, err := os.MkdirTemp("", "picoclaw-asr-test")
	if err != nil {
		http.Error(w, "temp dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		ext = ".wav"
	}
	dst := filepath.Join(tmpDir, "input"+ext)
	out, err := os.Create(dst)
	if err != nil {
		http.Error(w, "create file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		http.Error(w, "write file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	out.Close()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, "load config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var localCfg *config.LocalASRConfig
	if cfg.Voice.LocalASR != nil {
		localCfg = cfg.Voice.LocalASR
	} else if reqModel := strings.TrimSpace(r.FormValue("model_id")); reqModel != "" {
		if spec, ok := asrModelByID(reqModel); ok {
			localCfg = &config.LocalASRConfig{
				Enabled:    true,
				BinaryPath: h.sherpaBinaryPath(),
				ModelDir:   filepath.Join(h.asrModelsDir(), spec.DirName),
				ModelType:  spec.ModelType,
			}
		}
	}
	if localCfg == nil {
		http.Error(w, "local ASR is not configured", http.StatusBadRequest)
		return
	}

	tr, err := local.New(localCfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	audioSeconds := probeAudioDuration(dst)

	start := time.Now()
	res, err := tr.Transcribe(ctx, dst)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	elapsed := time.Since(start)

	resp := map[string]any{
		"text":          res.Text,
		"elapsed_ms":    elapsed.Milliseconds(),
		"audio_seconds": audioSeconds,
	}
	// Real-time factor: how many seconds of audio are transcribed per second of
	// compute. Lower is better; <1 means faster than real time.
	if audioSeconds > 0 {
		resp["rtf"] = math.Round(elapsed.Seconds()/audioSeconds*1000) / 1000
	}
	json.NewEncoder(w).Encode(resp)
}

// ---------------------------------------------------------------------------
// Instance registry
//
// The ASR install is machine-wide, so an uninstall can break other running
// instances. Every instance that points at the shared install records itself
// here (config path + pid). Uninstall checks liveness before deleting.
// ---------------------------------------------------------------------------

const asrRegistryFile = "instances.json"

type asrInstanceRecord struct {
	ConfigPath string `json:"config_path"`
	PID        int    `json:"pid"`
	UpdatedAt  string `json:"updated_at"`
}

func (h *Handler) registryPath() string {
	return filepath.Join(GlobalASRDir(), asrRegistryFile)
}

// registerInstance records this instance as a user of the shared install.
func (h *Handler) registerInstance() {
	path := h.registryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	records := h.readRegistry()
	now := time.Now().Format(time.RFC3339)

	cfgAbs, _ := filepath.Abs(h.configPath)
	found := false
	for i := range records {
		if records[i].ConfigPath == cfgAbs {
			records[i].PID = os.Getpid()
			records[i].UpdatedAt = now
			found = true
			break
		}
	}
	if !found {
		records = append(records, asrInstanceRecord{
			ConfigPath: cfgAbs, PID: os.Getpid(), UpdatedAt: now,
		})
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

func (h *Handler) readRegistry() []asrInstanceRecord {
	data, err := os.ReadFile(h.registryPath())
	if err != nil {
		return nil
	}
	var records []asrInstanceRecord
	if json.Unmarshal(data, &records) != nil {
		return nil
	}
	return records
}

// liveInstances returns the records whose process is still alive, excluding the
// caller itself. Dead entries are pruned from the file.
func (h *Handler) liveInstances() []asrInstanceRecord {
	records := h.readRegistry()
	self, _ := filepath.Abs(h.configPath)
	live := make([]asrInstanceRecord, 0, len(records))
	for _, rec := range records {
		if rec.ConfigPath == self {
			continue
		}
		if processAlive(rec.PID) {
			live = append(live, rec)
		}
	}
	// Persist the pruned list (plus ourselves).
	kept := live
	if self != "" {
		kept = append(kept, asrInstanceRecord{
			ConfigPath: self, PID: os.Getpid(),
			UpdatedAt: time.Now().Format(time.RFC3339),
		})
	}
	if data, err := json.MarshalIndent(kept, "", "  "); err == nil {
		_ = os.WriteFile(h.registryPath(), data, 0o644)
	}
	return live
}

func (h *Handler) handleLocalASRUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ModelID string `json:"model_id"`
		Runtime bool   `json:"runtime"`  // remove the sherpa-onnx binary
		All     bool   `json:"all"`      // remove everything
		Force   bool   `json:"force"`    // proceed even if other instances are alive
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	// The install is machine-wide: refuse to delete while other instances use it.
	if !req.Force {
		if live := h.liveInstances(); len(live) > 0 {
			json.NewEncoder(w).Encode(map[string]any{
				"ok":        false,
				"in_use":    true,
				"instances": live,
				"message": fmt.Sprintf(
					"%d other PicoClaw instance(s) are using this ASR install. "+
						"Stop them first, or confirm to delete anyway.", len(live)),
			})
			return
		}
	}

	switch {
	case req.All:
		if err := os.RemoveAll(h.asrRoot()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case req.Runtime:
		// Remove only the runtime binaries, keep downloaded models.
		if err := os.RemoveAll(h.asrBinDir()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case req.ModelID != "":
		if spec, ok := asrModelByID(req.ModelID); ok {
			_ = os.RemoveAll(filepath.Join(h.asrModelsDir(), spec.DirName))
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// Download / extract helpers
// ---------------------------------------------------------------------------

func (h *Handler) installSherpaBinary(url, dirPrefix string, st *asrTaskState) error {
	if err := os.MkdirAll(h.asrBinDir(), 0o755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "picoclaw-sherpa")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, "sherpa.tar.bz2")
	if err := h.downloadWithProgress(url, archivePath, st); err != nil {
		return err
	}

	st.set("extracting", "解压中…", 0)
	if err := extractTarBz2(archivePath, dirPrefix, h.asrBinDir(), true); err != nil {
		return err
	}
	return ensureExecutable(h.sherpaBinaryPath())
}

func (h *Handler) installModel(spec asrModelSpec, st *asrTaskState) error {
	if err := os.MkdirAll(h.asrModelsDir(), 0o755); err != nil {
		return fmt.Errorf("create models dir: %w", err)
	}
	target := filepath.Join(h.asrModelsDir(), spec.DirName)
	if _, err := os.Stat(target); err == nil {
		return nil // already downloaded
	}

	tmpDir, err := os.MkdirTemp("", "picoclaw-model")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, "model.tar.bz2")
	if err := h.downloadWithProgress(spec.ArchiveURL, archivePath, st); err != nil {
		return err
	}

	st.set("extracting", "解压中…", 0)
	return extractTarBz2(archivePath, spec.DirName, target, false)
}

// downloadWithProgress streams url to dst while updating st.
func (h *Handler) downloadWithProgress(url, dst string, st *asrTaskState) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	// Use a transport that honours HTTP(S)_PROXY / ALL_PROXY from the
	// environment, so users behind a proxy can still download.
	client := &http.Client{
		Timeout:   0,
		Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d from %s", resp.StatusCode, url)
	}

	total := resp.ContentLength
	st.mu.Lock()
	st.bytesTotal = total
	st.mu.Unlock()

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 128*1024)
	var done int64
	lastUpdate := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			if time.Since(lastUpdate) > 200*time.Millisecond {
				lastUpdate = time.Now()
				pct := 0.0
				if total > 0 {
					pct = float64(done) / float64(total) * 100
				}
				st.mu.Lock()
				st.bytesDone = done
				st.percent = pct
				st.message = fmt.Sprintf("下载中 %s / %s", humanBytes(done), humanBytes(total))
				st.mu.Unlock()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	st.mu.Lock()
	st.bytesDone = done
	st.percent = 100
	st.mu.Unlock()
	return nil
}

// extractTarBz2 extracts a .tar.bz2 into destDir.
//
// Archives from k2-fsa are laid out as "<dirPrefix>/<rest>". Two shapes exist:
//   - runtime: "<prefix>/bin/sherpa-onnx-offline.exe" and "<prefix>/lib/*.dll"
//     -> binOnly=true: strip "<prefix>/bin/" so the executable and its DLLs sit
//        directly in destDir (sherpa-onnx resolves DLLs next to the exe).
//   - model:   "<prefix>/model.int8.onnx", "<prefix>/tokens.txt", ...
//     -> binOnly=false: strip "<prefix>/" so the model files land in destDir.
func extractTarBz2(archivePath, dirPrefix, destDir string, binOnly bool) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	prefix := strings.TrimSuffix(filepath.ToSlash(dirPrefix), "/")
	stripBin := prefix + "/bin/"
	stripRoot := prefix + "/"

	tr := tar.NewReader(bzip2.NewReader(f))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		name := filepath.ToSlash(hdr.Name)

		var rel string
		if binOnly {
			// Windows resolves DLLs next to the executable, so bin/* and lib/*
			// are flattened together into destDir. On Linux the loader uses
			// LD_LIBRARY_PATH (set at exec time), so the same flat layout works.
			rel = strings.TrimPrefix(name, stripRoot)
			if rel == name {
				continue
			}
			switch {
			case strings.HasPrefix(rel, "bin/"):
				rel = strings.TrimPrefix(rel, "bin/")
			case strings.HasPrefix(rel, "lib/"):
				rel = strings.TrimPrefix(rel, "lib/")
			default:
				continue
			}
		} else {
			rel = strings.TrimPrefix(name, stripRoot)
			if rel == name {
				continue
			}
		}
		_ = stripBin

		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || strings.HasPrefix(rel, "..") {
			continue
		}
		outPath := filepath.Join(destDir, filepath.FromSlash(rel))
		if hdr.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(outPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		out.Close()
	}
	return nil
}

// ensureExecutable marks the binary runnable on non-Windows platforms. The
// archive does not always preserve the mode bit, and without it exec fails with
// "permission denied".
func ensureExecutable(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}

func humanBytes(n int64) string {
	if n <= 0 {
		return "?"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// asrTasks lazily creates the shared task manager on the handler.
func (h *Handler) asrTasks() *asrTaskManager {
	h.asrTaskOnce.Do(func() {
		h.asrTaskMgr = newASRTaskManager()
	})
	return h.asrTaskMgr
}

