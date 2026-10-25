package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/sticker"
)

const (
	// PromptSourceStickerList is the prompt source ID for sticker list injection
	PromptSourceStickerList PromptSourceID = "runtime.sticker_list"
)

// stickerPromptContributor injects the available sticker list into the system prompt
type stickerPromptContributor struct{}

// PromptSource returns the descriptor for this prompt contributor
func (c stickerPromptContributor) PromptSource() PromptSourceDescriptor {
	return PromptSourceDescriptor{
		ID:              PromptSourceStickerList,
		Owner:           "sticker",
		Description:     "Dynamic list of available Telegram stickers for AI to send",
		Allowed:         []PromptPlacement{{Layer: PromptLayerContext, Slot: PromptSlotWorkspace}},
		StableByDefault: false,
	}
}

// ContributePrompt generates the sticker list prompt part
func (c stickerPromptContributor) ContributePrompt(ctx context.Context, req PromptBuildRequest) ([]PromptPart, error) {
	logger.InfoCF("sticker", "StickerPromptContributor called", map[string]any{
		"channel": req.Channel,
	})

	// Only inject for Telegram channel - check contains "telegram" (case-insensitive)
	channelLower := strings.ToLower(req.Channel)
	if !strings.Contains(channelLower, "telegram") && !strings.Contains(channelLower, "tg") {
		logger.InfoCF("sticker", "Skipping sticker prompt - not Telegram channel", map[string]any{
			"channel": req.Channel,
		})
		return nil, nil
	}

	store := sticker.NewStore()
	stickers := store.GetAll()
	logger.InfoCF("sticker", "Loaded stickers for prompt", map[string]any{
		"count":   len(stickers),
		"channel": req.Channel,
	})
	if len(stickers) == 0 {
		logger.InfoCF("sticker", "No stickers found in store", map[string]any{
			"channel": req.Channel,
		})
		return nil, nil
	}

	var sb strings.Builder
	sb.WriteString("\n\n你现在可以使用以下 Telegram 自定义贴纸/表情包来让你的回答更加生动有趣。\n")
	sb.WriteString("如果你认为在当前的对话语境下发送某个表情包非常合适，请调用 send_sticker 工具（参数 sticker_id 填下方列表中的 StickerID），不要在回复正文里写任何标记文本。\n")
	sb.WriteString("注意：\n")
	sb.WriteString("1. 每次回复最多只能调用 1 次 send_sticker。\n")
	sb.WriteString("2. 只有在确实非常契合、幽默或者需要强烈表达情绪时才使用，不要频繁滥用。\n")
	sb.WriteString("3. 只能使用下方列表中原样给出的 StickerID（下划线、连字符都是 ID 的一部分，必须一字不差地照抄）。\n")
	sb.WriteString("4. 调用工具后请照常输出文本回复；工具会返回成功或失败，若失败请如实告知用户，不要谎称已发送。\n\n")
	sb.WriteString("当前可用表情包列表：\n")

	for _, item := range stickers {
		emojiHint := item.EmojiHint
		if emojiHint == "" {
			emojiHint = "无"
		}
		desc := item.Description
		if strings.TrimSpace(desc) == "" {
			// No visual description available (model has no vision, or it was not
			// generated). Say so plainly so the model judges from the emoji
			// instead of assuming a generic description.
			desc = "（无画面描述，请仅依据 Emoji 判断情绪）"
		}
		sb.WriteString(fmt.Sprintf("- StickerID: \"%s\" | Emoji: %s | 适用场景: \"%s\" | 描述: \"%s\"\n",
			item.ID, emojiHint, item.UsageScenarios, desc))
	}

	return []PromptPart{
		{
			ID:      "sticker.list",
			Layer:   PromptLayerContext,
			Slot:    PromptSlotWorkspace,
			Source:  PromptSource{ID: PromptSourceStickerList, Name: "sticker.list"},
			Title:   "Available Telegram Stickers",
			Content: sb.String(),
			Stable:  false,
			Cache:   PromptCacheNone,
		},
	}, nil
}
