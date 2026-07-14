package sticker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

const (
	stickerDBFileName = "telegram_stickers.json"
	mediaSubDir       = "media/stickers"
)

// Store 是表情包的持久化存储，使用 sync.RWMutex 保证并发安全
type Store struct {
	mu       sync.RWMutex
	db       StickerDatabase
	filePath string
	mediaDir string
}

// NewStore 创建一个新的表情包存储实例
func NewStore() *Store {
	home := config.GetHome()
	filePath := filepath.Join(home, stickerDBFileName)
	mediaDir := filepath.Join(home, mediaSubDir)

	// 确保媒体目录存在
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		logger.WarnCF("sticker", "Failed to create media directory", map[string]any{
			"path":  mediaDir,
			"error": err.Error(),
		})
	}

	s := &Store{
		filePath: filePath,
		mediaDir: mediaDir,
	}
	s.load()
	return s
}

// load 从 JSON 文件加载数据
func (s *Store) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.db = StickerDatabase{Stickers: []StickerItem{}}
			return
		}
		logger.WarnCF("sticker", "Failed to load sticker database", map[string]any{
			"error": err.Error(),
		})
		s.db = StickerDatabase{Stickers: []StickerItem{}}
		return
	}

	if err := json.Unmarshal(data, &s.db); err != nil {
		logger.WarnCF("sticker", "Failed to parse sticker database", map[string]any{
			"error": err.Error(),
		})
		s.db = StickerDatabase{Stickers: []StickerItem{}}
		return
	}

	if s.db.Stickers == nil {
		s.db.Stickers = []StickerItem{}
	}
}

// save 将数据持久化到 JSON 文件
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.db, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sticker database: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write sticker database: %w", err)
	}

	return nil
}

// GetAll 获取所有表情包
func (s *Store) GetAll() []StickerItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]StickerItem, len(s.db.Stickers))
	copy(result, s.db.Stickers)
	return result
}

// GetByID 根据 ID 获取单个表情包
func (s *Store) GetByID(id string) (StickerItem, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, item := range s.db.Stickers {
		if item.ID == id {
			return item, true
		}
	}
	return StickerItem{}, false
}

// Add 添加新的表情包
func (s *Store) Add(item StickerItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查 ID 是否已存在
	for _, existing := range s.db.Stickers {
		if existing.ID == item.ID {
			return fmt.Errorf("sticker ID %q already exists", item.ID)
		}
	}

	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now()
	}

	s.db.Stickers = append(s.db.Stickers, item)
	return s.save()
}

// Update 更新表情包信息
func (s *Store) Update(item StickerItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.db.Stickers {
		if existing.ID == item.ID {
			s.db.Stickers[i] = item
			return s.save()
		}
	}
	return fmt.Errorf("sticker ID %q not found", item.ID)
}

// UpdateTelegramFileID 更新指定表情包的 Telegram FileID
func (s *Store) UpdateTelegramFileID(id, fileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, item := range s.db.Stickers {
		if item.ID == id {
			s.db.Stickers[i].TelegramFileID = fileID
			return s.save()
		}
	}
	return fmt.Errorf("sticker ID %q not found", id)
}

// Delete 删除指定表情包并清理本地文件
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, item := range s.db.Stickers {
		if item.ID == id {
			// 物理删除本地缓存图片
			if item.FilePath != "" {
				if err := os.Remove(item.FilePath); err != nil && !os.IsNotExist(err) {
					logger.WarnCF("sticker", "Failed to delete sticker file", map[string]any{
						"path":  item.FilePath,
						"error": err.Error(),
					})
				}
			}

			s.db.Stickers = append(s.db.Stickers[:i], s.db.Stickers[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("sticker ID %q not found", id)
}

// GetMediaDir 返回媒体文件存储目录
func (s *Store) GetMediaDir() string {
	return s.mediaDir
}
