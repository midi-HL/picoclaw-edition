package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mymmrac/telego"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/sticker"
)

// StickerAPIHandler handles REST API endpoints for sticker management.
type StickerAPIHandler struct {
	store *sticker.Store
	cfg   *config.Config
}

// NewStickerAPIHandler creates a new StickerAPIHandler.
func NewStickerAPIHandler(store *sticker.Store, cfg *config.Config) *StickerAPIHandler {
	return &StickerAPIHandler{
		store: store,
		cfg:   cfg,
	}
}

// RegisterRoutes registers all sticker-related HTTP routes on the given mux.
func (h *StickerAPIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/telegram/stickers", h.handleGetStickers)
	mux.HandleFunc("/api/telegram/stickers/manual", h.handleManualUpload)
	mux.HandleFunc("/api/telegram/stickers/import-set", h.handleImportSet)
	mux.HandleFunc("/api/telegram/stickers/", h.handleDeleteSticker)
	mux.HandleFunc("/api/media/file", h.handleStickerMediaFile)
}

// handleStickerMediaFile serves a locally stored sticker image for preview.
// Only files inside the sticker media directory are served.
func (h *StickerAPIHandler) handleStickerMediaFile(w http.ResponseWriter, r *http.Request) {
	rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if rawPath == "" {
		writeJSONError(w, http.StatusBadRequest, "path is required")
		return
	}

	mediaDir, err := filepath.Abs(h.store.GetMediaDir())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "media dir unavailable")
		return
	}
	absPath, err := filepath.Abs(rawPath)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}
	rel, err := filepath.Rel(mediaDir, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		writeJSONError(w, http.StatusForbidden, "path outside sticker media directory")
		return
	}

	f, err := os.Open(absPath)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "file not found")
		return
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.IsDir() {
		writeJSONError(w, http.StatusBadRequest, "path is a directory")
		return
	}

	w.Header().Set("Content-Type", "image/webp")
	http.ServeContent(w, r, filepath.Base(absPath), time.Time{}, f)
}

// handleGetStickers handles GET /api/telegram/stickers
func (h *StickerAPIHandler) handleGetStickers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	stickers := h.store.GetAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"stickers": stickers})
}

// stickerImportRequest is the request body for import-set endpoint.
type stickerImportRequest struct {
	StickerSetName string `json:"sticker_set_name"`
}

// handleImportSet handles POST /api/telegram/stickers/import-set
func (h *StickerAPIHandler) handleImportSet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req stickerImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	setName := strings.TrimSpace(req.StickerSetName)
	if setName == "" {
		writeJSONError(w, http.StatusBadRequest, "sticker_set_name is required")
		return
	}
	if !sticker.IsSafeID(setName) {
		writeJSONError(w, http.StatusBadRequest, "invalid sticker set name")
		return
	}

	logger.InfoCF("sticker", "Importing sticker set", map[string]any{
		"set_name": setName,
	})

	// Get Telegram token from config
	tgToken := h.getTelegramToken()
	if tgToken == "" {
		writeJSONError(w, http.StatusBadRequest, "Telegram token not configured")
		return
	}

	// Create a temporary bot for fetching sticker set
	bot, err := telego.NewBot(tgToken)
	if err != nil {
		logger.ErrorCF("sticker", "Failed to create Telegram bot", map[string]any{
			"error": err.Error(),
		})
		writeJSONError(w, http.StatusInternalServerError, "failed to create Telegram bot: "+err.Error())
		return
	}

	ctx := r.Context()

	// Fetch the sticker set
	set, err := bot.GetStickerSet(ctx, &telego.GetStickerSetParams{
		Name: setName,
	})
	if err != nil {
		logger.ErrorCF("sticker", "Failed to get sticker set", map[string]any{
			"set_name": setName,
			"error":    err.Error(),
		})
		writeJSONError(w, http.StatusInternalServerError, "failed to get sticker set: "+err.Error())
		return
	}

	// Get default LLM provider for auto-describing
	provider, _, providerErr := providers.CreateProvider(h.cfg)

	imported := 0
	failed := 0
	var lastErr string
	for idx, tgSticker := range set.Stickers {
		// Use the sticker's position in the set for the ID so IDs stay stable and
		// unique even when some entries are skipped or fail.
		stickerID := fmt.Sprintf("%s_%d", setName, idx)

		// Skip if sticker ID already exists
		if _, exists := h.store.GetByID(stickerID); exists {
			continue
		}

		// Determine file ID and extension based on format
		var targetFileID string
		var ext string
		if (tgSticker.IsAnimated || tgSticker.IsVideo) && tgSticker.Thumbnail != nil {
			targetFileID = tgSticker.Thumbnail.FileID
			ext = ".jpg"
		} else {
			targetFileID = tgSticker.FileID
			ext = ".webp"
		}

		// Download the sticker file
		localPath, err := h.downloadStickerFile(bot, ctx, targetFileID, ext, setName, idx)
		if err != nil {
			failed++
			lastErr = err.Error()
			logger.WarnCF("sticker", "Failed to download sticker", map[string]any{
				"set":   setName,
				"index": idx,
				"error": err.Error(),
			})
			continue
		}

		// Auto-generate description using default LLM
		description := ""
		usageScenarios := "适用于日常聊天中的相关场景"
		if providerErr == nil && provider != nil {
			description = h.autoDescribeSticker(ctx, provider, localPath)
		}

		// Build emoji hint
		emojiHint := ""
		if tgSticker.Emoji != "" {
			emojiHint = tgSticker.Emoji
		}

		item := sticker.StickerItem{
			ID:             stickerID,
			SourceType:     sticker.SourceTelegramSet,
			StickerSetName: setName,
			FilePath:       localPath,
			TelegramFileID: tgSticker.FileID,
			EmojiHint:      emojiHint,
			Description:    description,
			UsageScenarios: usageScenarios,
		}

		if err := h.store.Add(item); err != nil {
			failed++
			lastErr = err.Error()
			logger.WarnCF("sticker", "Failed to add sticker to store", map[string]any{
				"id":    stickerID,
				"error": err.Error(),
			})
			continue
		}

		imported++
	}

	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"imported": imported,
		"total":    len(set.Stickers),
		"set_name": setName,
	}
	if failed > 0 {
		resp["failed"] = failed
		resp["error"] = fmt.Sprintf("%d/%d 张贴纸导入失败：%s", failed, len(set.Stickers), lastErr)
	}
	json.NewEncoder(w).Encode(resp)
}

// handleManualUpload handles POST /api/telegram/stickers/manual
func (h *StickerAPIHandler) handleManualUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB max
		writeJSONError(w, http.StatusBadRequest, "failed to parse form: "+err.Error())
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	description := strings.TrimSpace(r.FormValue("description"))
	usageScenarios := strings.TrimSpace(r.FormValue("usage_scenarios"))
	emojiHint := strings.TrimSpace(r.FormValue("emoji_hint"))

	if id == "" || description == "" || usageScenarios == "" {
		writeJSONError(w, http.StatusBadRequest, "id, description, and usage_scenarios are required")
		return
	}

	if !sticker.IsSafeID(id) {
		writeJSONError(w, http.StatusBadRequest, "id may only contain letters, digits, '-' and '_'")
		return
	}

	// Check if ID already exists
	if _, exists := h.store.GetByID(id); exists {
		writeJSONError(w, http.StatusConflict, "sticker ID already exists")
		return
	}

	// Get the uploaded file
	file, handler, err := r.FormFile("file")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to get file: "+err.Error())
		return
	}
	defer file.Close()

	// Determine file extension
	ext := filepath.Ext(handler.Filename)
	if ext == "" {
		ext = ".webp"
	}

	// Save file to media directory
	mediaDir := h.store.GetMediaDir()
	filePath := filepath.Join(mediaDir, id+ext)

	dst, err := os.Create(filePath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save file: "+err.Error())
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to write file: "+err.Error())
		return
	}

	item := sticker.StickerItem{
		ID:             id,
		SourceType:     sticker.SourceManual,
		FilePath:       filePath,
		EmojiHint:      emojiHint,
		Description:    description,
		UsageScenarios: usageScenarios,
	}

	if err := h.store.Add(item); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save sticker: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": id})
}

// handleDeleteSticker handles DELETE /api/telegram/stickers/delete?id=xxx
func (h *StickerAPIHandler) handleDeleteSticker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		// Try parsing from request body for POST
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			id = req.ID
		}
	}

	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "sticker ID is required")
		return
	}

	if err := h.store.Delete(id); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"deleted": id})
}

// getTelegramToken retrieves the Telegram bot token from config.
func (h *StickerAPIHandler) getTelegramToken() string {
	tgChannel := h.cfg.Channels.Get(config.ChannelTelegram)
	if tgChannel == nil {
		return ""
	}

	var settings config.TelegramSettings
	if err := tgChannel.Settings.Decode(&settings); err != nil {
		logger.WarnCF("sticker", "Failed to decode Telegram settings", map[string]any{
			"error": err.Error(),
		})
		return ""
	}

	return settings.Token.String()
}

// downloadStickerFile downloads a sticker file from Telegram.
func (h *StickerAPIHandler) downloadStickerFile(bot *telego.Bot, ctx context.Context, fileID, ext, setName string, index int) (string, error) {
	file, err := bot.GetFile(ctx, &telego.GetFileParams{FileID: fileID})
	if err != nil {
		return "", fmt.Errorf("failed to get file: %w", err)
	}

	if file.FilePath == "" {
		return "", fmt.Errorf("empty file path")
	}

	// Download the file
	url := bot.FileDownloadURL(file.FilePath)
	tmpDir := filepath.Join(os.TempDir(), "picoclaw_stickers")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	filename := fmt.Sprintf("%s_%d%s", setName, index, ext)
	dstPath := filepath.Join(tmpDir, filename)

	// Download using HTTP
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	dst, err := os.Create(dstPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, resp.Body); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	// Copy to persistent media directory
	mediaDir := h.store.GetMediaDir()
	persistentPath := filepath.Join(mediaDir, filename)
	if err := copyFile(dstPath, persistentPath); err != nil {
		return "", fmt.Errorf("failed to copy to media dir: %w", err)
	}

	return persistentPath, nil
}

// autoDescribeSticker uses the default LLM to generate a description for a sticker.
func (h *StickerAPIHandler) autoDescribeSticker(ctx context.Context, provider providers.LLMProvider, imagePath string) string {
	dataURL, err := sticker.FileToDataURL(imagePath)
	if err != nil {
		logger.WarnCF("sticker", "Failed to read image for description", map[string]any{
			"path":  imagePath,
			"error": err.Error(),
		})
		return ""
	}

	prompt := "你是一个表情包分析助手。请用一两句简练、生动、客观的话，描述这张表情包中角色的形象、表情动作、所传达的情绪，以及它适合用在什么聊天场景或语境中。直接返回描述，不要任何多余修饰。"

	messages := []providers.Message{
		{
			Role:    "user",
			Content: prompt,
			Media:   []string{dataURL},
		},
	}

	response, err := provider.Chat(ctx, messages, nil, "", nil)
	if err != nil {
		logger.WarnCF("sticker", "Failed to generate sticker description", map[string]any{
			"error": err.Error(),
		})
		return ""
	}

	return strings.TrimSpace(response.Content)
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
