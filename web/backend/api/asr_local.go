package api

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
}

// asrRoot is <picoclaw-home>/asr.
func asrRoot() string {
	return filepath.Join(config.GetHome(), "asr")
}

func asrBinDir() string    { return filepath.Join(asrRoot(), "bin") }
func asrModelsDir() string { return filepath.Join(asrRoot(), "models") }

func sherpaBinaryName() string {
	if runtime.GOOS == "windows" {
		return "sherpa-onnx-offline.exe"
	}
	return "sherpa-onnx-offline"
}

func sherpaBinaryPath() string { return filepath.Join(asrBinDir(), sherpaBinaryName()) }

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
		return fmt.Sprintf("https://github.com/k2-fsa/sherpa-onnx/releases/download/%s/sherpa-onnx-%s-linux-aarch64-shared.tar.bz2", sherpaRelease, sherpaRelease),
			fmt.Sprintf("sherpa-onnx-%s-linux-aarch64-shared", sherpaRelease), true
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
	if _, err := os.Stat(sherpaBinaryPath()); err == nil {
		installed = true
	}

	downloaded := []string{}
	if entries, err := os.ReadDir(asrModelsDir()); err == nil {
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
		"binary_path":        sherpaBinaryPath(),
		"models_dir":         asrModelsDir(),
		"downloaded":         downloaded,
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
	for _, m := range asrModels {
		dir := filepath.Join(asrModelsDir(), m.DirName)
		_, err := os.Stat(dir)
		out = append(out, item{
			asrModelSpec: m,
			Downloaded:   err == nil,
			Active:       m.ID == activeID,
		})
	}
	json.NewEncoder(w).Encode(map[string]any{"models": out})
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
	modelDir := filepath.Join(asrModelsDir(), spec.DirName)
	if _, err := os.Stat(modelDir); err != nil {
		http.Error(w, "model not downloaded yet", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(sherpaBinaryPath()); err != nil {
		http.Error(w, "sherpa-onnx not installed yet", http.StatusBadRequest)
		return
	}

	cfg.Voice.ASRSource = "local"
	cfg.Voice.LocalASR = &config.LocalASRConfig{
		Enabled:    true,
		BinaryPath: sherpaBinaryPath(),
		ModelDir:   modelDir,
		ModelID:    spec.ID,
		ModelType:  spec.ModelType,
		Threads:    req.Threads,
	}

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, "save config: "+err.Error(), http.StatusInternalServerError)
		return
	}
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
				BinaryPath: sherpaBinaryPath(),
				ModelDir:   filepath.Join(asrModelsDir(), spec.DirName),
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

	start := time.Now()
	res, err := tr.Transcribe(ctx, dst)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"text":       res.Text,
		"elapsed_ms": time.Since(start).Milliseconds(),
	})
}

func (h *Handler) handleLocalASRUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ModelID string `json:"model_id"`
		All     bool   `json:"all"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.All {
		if err := os.RemoveAll(asrRoot()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if req.ModelID != "" {
		if spec, ok := asrModelByID(req.ModelID); ok {
			_ = os.RemoveAll(filepath.Join(asrModelsDir(), spec.DirName))
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// Download / extract helpers
// ---------------------------------------------------------------------------

func (h *Handler) installSherpaBinary(url, dirPrefix string, st *asrTaskState) error {
	if err := os.MkdirAll(asrBinDir(), 0o755); err != nil {
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
	return extractTarBz2(archivePath, dirPrefix, asrBinDir(), true)
}

func (h *Handler) installModel(spec asrModelSpec, st *asrTaskState) error {
	if err := os.MkdirAll(asrModelsDir(), 0o755); err != nil {
		return fmt.Errorf("create models dir: %w", err)
	}
	target := filepath.Join(asrModelsDir(), spec.DirName)
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

