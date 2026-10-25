package integrationtools

import (
	"context"
	"fmt"
	"strings"
)

// StickerSender sends a sticker identified by ID to a chat. Implementations
// resolve the ID against the sticker store and deliver it through the channel.
type StickerSender func(ctx context.Context, channel, chatID, stickerID string) error

// SendStickerTool lets the model send a Telegram sticker as a real tool call
// instead of embedding a text directive in its reply.
//
// Why a tool: the previous approach parsed a [SEND_STICKER: id] tag out of the
// model's text in the channel layer. That was fragile — outbound text is split
// into chunks before reaching the channel, so a tag near the 4000-rune boundary
// could be cut in half and silently dropped, and the model had no way to learn
// whether the sticker was actually delivered.
type SendStickerTool struct {
	sender StickerSender
}

func NewSendStickerTool() *SendStickerTool {
	return &SendStickerTool{}
}

func (t *SendStickerTool) Name() string {
	return "send_sticker"
}

func (t *SendStickerTool) Description() string {
	return "Telegram-only: send a sticker/emoji-pack image to the current Telegram chat. " +
		"Use this when a sticker expresses the reaction better than text. " +
		"Do NOT use it on other channels (Discord, WeChat, Feishu, pico, etc.) — they cannot send stickers. " +
		"Only sticker IDs from the available sticker list may be used. " +
		"Returns an error if the sticker could not be delivered, so you always " +
		"know whether it was actually sent."
}

func (t *SendStickerTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"sticker_id": map[string]any{
				"type":        "string",
				"description": "Exact sticker ID from the available sticker list (e.g. myadestes_1_nacho_42). Underscores and hyphens are significant.",
			},
			"channel": map[string]any{
				"type":        "string",
				"description": "Optional: target channel; defaults to the current channel",
			},
			"chat_id": map[string]any{
				"type":        "string",
				"description": "Optional: target chat ID; defaults to the current chat",
			},
		},
		"required": []string{"sticker_id"},
	}
}

func (t *SendStickerTool) SetSender(sender StickerSender) {
	t.sender = sender
}

func (t *SendStickerTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	stickerID, _ := args["sticker_id"].(string)
	stickerID = strings.TrimSpace(stickerID)
	if stickerID == "" {
		return &ToolResult{ForLLM: "sticker_id is required", IsError: true}
	}

	channel, _ := args["channel"].(string)
	chatID, _ := args["chat_id"].(string)
	if channel == "" {
		channel = ToolChannel(ctx)
	}
	if chatID == "" {
		chatID = ToolChatID(ctx)
	}
	if channel == "" || chatID == "" {
		return &ToolResult{ForLLM: "No target channel/chat available", IsError: true}
	}
	if t.sender == nil {
		return &ToolResult{ForLLM: "Sticker sending is not configured", IsError: true}
	}

	if err := t.sender(ctx, channel, chatID, stickerID); err != nil {
		// Give the model an actionable message so it does not retry blindly or
		// falsely claim success to the user.
		hint := ""
		if strings.Contains(err.Error(), "not found") {
			hint = " (the sticker ID must be copied exactly from the available sticker list, including underscores)"
		}
		return &ToolResult{
			ForLLM:  fmt.Sprintf("failed to send sticker %q: %v%s. Tell the user honestly that the sticker was NOT sent.", stickerID, err, hint),
			IsError: true,
			Err:     err,
		}
	}

	return &ToolResult{
		ForLLM: fmt.Sprintf("Sticker %q sent to %s:%s", stickerID, channel, chatID),
		Silent: true,
	}
}
