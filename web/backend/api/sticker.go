package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/sticker"
)

// StickerAPIHandler handles sticker management API endpoints in the launcher.
type StickerAPIHandler struct {
	store *sticker.Store
	cfg   *config.Config
}

// NewStickerAPIHandler creates a new StickerAPIHandler for the launcher.
func NewStickerAPIHandler(cfg *config.Config) *StickerAPIHandler {
	return &StickerAPIHandler{
		store: sticker.NewStore(),
		cfg:   cfg,
	}
}

// RegisterStickerRoutes registers sticker management routes on the mux.
func (h *StickerAPIHandler) RegisterStickerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/telegram/stickers", h.handleGetStickers)
	mux.HandleFunc("POST /api/telegram/stickers/manual", h.handleManualUpload)
	mux.HandleFunc("POST /api/telegram/stickers/import-set", h.handleImportSet)
	mux.HandleFunc("DELETE /api/telegram/stickers/delete", h.handleDeleteSticker)
}

// handleGetStickers handles GET /api/telegram/stickers
func (h *StickerAPIHandler) handleGetStickers(w http.ResponseWriter, r *http.Request) {
	stickers := h.store.GetAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"stickers": stickers})
}

// handleManualUpload handles POST /api/telegram/stickers/manual
func (h *StickerAPIHandler) handleManualUpload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeStickerError(w, http.StatusBadRequest, "failed to parse form: "+err.Error())
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	description := strings.TrimSpace(r.FormValue("description"))
	usageScenarios := strings.TrimSpace(r.FormValue("usage_scenarios"))
	emojiHint := strings.TrimSpace(r.FormValue("emoji_hint"))

	if id == "" || description == "" || usageScenarios == "" {
		writeStickerError(w, http.StatusBadRequest, "id, description, and usage_scenarios are required")
		return
	}

	if _, exists := h.store.GetByID(id); exists {
		writeStickerError(w, http.StatusConflict, "sticker ID already exists")
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		writeStickerError(w, http.StatusBadRequest, "failed to get file: "+err.Error())
		return
	}
	defer file.Close()

	ext := filepath.Ext(handler.Filename)
	if ext == "" {
		ext = ".webp"
	}

	mediaDir := h.store.GetMediaDir()
	filePath := filepath.Join(mediaDir, id+ext)

	dst, err := os.Create(filePath)
	if err != nil {
		writeStickerError(w, http.StatusInternalServerError, "failed to save file: "+err.Error())
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		writeStickerError(w, http.StatusInternalServerError, "failed to write file: "+err.Error())
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
		writeStickerError(w, http.StatusInternalServerError, "failed to save sticker: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": id})
}

// handleImportSet handles POST /api/telegram/stickers/import-set
func (h *StickerAPIHandler) handleImportSet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		writeStickerError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		StickerSetName string `json:"sticker_set_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeStickerError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	setName := strings.TrimSpace(req.StickerSetName)
	if setName == "" {
		writeStickerError(w, http.StatusBadRequest, "sticker_set_name is required")
		return
	}

	logger.InfoCF("sticker", "Importing sticker set (launcher)", map[string]any{
		"set_name": setName,
	})

	// Note: The actual Telegram API call should be made by the gateway.
	// For now, return a message indicating the import is pending.
	// In a full implementation, this would proxy to the gateway or
	// use the Telegram API directly.

	writeStickerError(w, http.StatusNotImplemented, "Import via launcher requires gateway integration. Please use the gateway API directly.")
}

// handleDeleteSticker handles DELETE /api/telegram/stickers/delete
func (h *StickerAPIHandler) handleDeleteSticker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id := r.URL.Query().Get("id")
	if id == "" {
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			id = req.ID
		}
	}

	if id == "" {
		writeStickerError(w, http.StatusBadRequest, "sticker ID is required")
		return
	}

	if err := h.store.Delete(id); err != nil {
		writeStickerError(w, http.StatusNotFound, err.Error())
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"deleted": id})
}

func writeStickerError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
