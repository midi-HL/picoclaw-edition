package sticker

import "time"

// StickerSource 表示表情包的来源类型
type StickerSource string

const (
	SourceManual      StickerSource = "manual"        // 模式 A: 手动录入
	SourceTelegramSet StickerSource = "telegram_set"  // 模式 B: 官方贴纸集导入
)

// StickerItem 表示单个表情包的元数据
type StickerItem struct {
	ID             string        `json:"id"`                         // 表情包唯一标识
	SourceType     StickerSource `json:"source_type"`                // 来源类型
	StickerSetName string        `json:"sticker_set_name,omitempty"` // 贴纸集包名 (若为导入)
	FilePath       string        `json:"file_path"`                  // 本地缓存图片路径
	TelegramFileID string        `json:"telegram_file_id,omitempty"` // Telegram 服务器上的 file_id
	EmojiHint      string        `json:"emoji_hint,omitempty"`       // 关联的快捷 Emoji (可选)
	Description    string        `json:"description"`                // 画面详细描述
	UsageScenarios string        `json:"usage_scenarios"`            // 适用场景
	CreatedAt      time.Time     `json:"created_at"`                 // 创建时间
}

// StickerDatabase 是表情包数据库的顶层结构
type StickerDatabase struct {
	Stickers []StickerItem `json:"stickers"`
}
