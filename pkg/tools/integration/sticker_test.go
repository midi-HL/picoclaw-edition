package integrationtools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSendStickerTool_RequiresStickerID(t *testing.T) {
	tool := NewSendStickerTool()
	tool.SetSender(func(context.Context, string, string, string) error { return nil })

	ctx := WithToolContext(context.Background(), "telegram", "123")
	res := tool.Execute(ctx, map[string]any{})
	if res == nil || !res.IsError {
		t.Fatalf("expected error when sticker_id is missing, got %#v", res)
	}
}

func TestSendStickerTool_DelegatesToSender(t *testing.T) {
	var gotChannel, gotChat, gotSticker string
	tool := NewSendStickerTool()
	tool.SetSender(func(_ context.Context, channel, chatID, stickerID string) error {
		gotChannel, gotChat, gotSticker = channel, chatID, stickerID
		return nil
	})

	ctx := WithToolContext(context.Background(), "telegram", "123/45")
	res := tool.Execute(ctx, map[string]any{"sticker_id": "myadestes_1_nacho_42"})
	if res == nil || res.IsError {
		t.Fatalf("expected success, got %#v", res)
	}
	if gotChannel != "telegram" || gotChat != "123/45" || gotSticker != "myadestes_1_nacho_42" {
		t.Fatalf("sender args = %q/%q/%q", gotChannel, gotChat, gotSticker)
	}
	if !res.Silent {
		t.Fatal("expected Silent result so the tool does not post a user-facing message")
	}
}

func TestSendStickerTool_ReportsFailureToModel(t *testing.T) {
	tool := NewSendStickerTool()
	tool.SetSender(func(context.Context, string, string, string) error {
		return errors.New(`sticker "nope" not found`)
	})

	ctx := WithToolContext(context.Background(), "telegram", "123")
	res := tool.Execute(ctx, map[string]any{"sticker_id": "nope"})
	if res == nil || !res.IsError {
		t.Fatalf("expected error result, got %#v", res)
	}
	// The model must be told the sticker was NOT sent, so it does not claim success.
	if !strings.Contains(res.ForLLM, "NOT sent") {
		t.Fatalf("expected failure to be explicit, got %q", res.ForLLM)
	}
	if !strings.Contains(res.ForLLM, "underscores") {
		t.Fatalf("expected ID hint for not-found errors, got %q", res.ForLLM)
	}
}

func TestSendStickerTool_NotConfigured(t *testing.T) {
	tool := NewSendStickerTool()
	ctx := WithToolContext(context.Background(), "telegram", "123")
	res := tool.Execute(ctx, map[string]any{"sticker_id": "x"})
	if res == nil || !res.IsError {
		t.Fatalf("expected error when sender is not configured, got %#v", res)
	}
}
