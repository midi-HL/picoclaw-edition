package telegram

import "testing"

// Stickers are now delivered through the send_sticker tool. The channel only
// strips legacy [SEND_STICKER: id] markers left over from older conversations so
// users never see the raw directive. These tests lock in that stripping behaviour
// (and that it never mangles unrelated text).
func TestStripLegacyStickerTag(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantText string
	}{
		{
			name:     "simple id",
			input:    "好的，给你一个 [SEND_STICKER: happy_dog]",
			wantText: "好的，给你一个",
		},
		{
			name:     "id with multiple underscores and digits",
			input:    "[SEND_STICKER: myadestes_1_nacho_5]",
			wantText: "",
		},
		{
			name:     "markdown-stripped tag (underscores lost)",
			input:    "发出去了没？[SENDSTICKER: myadestes1nacho5]",
			wantText: "发出去了没？",
		},
		{
			name:     "with surrounding spaces",
			input:    "hi [ SEND_STICKER :  abc_def ] bye",
			wantText: "hi  bye",
		},
		{
			name:     "no tag",
			input:    "just a normal reply",
			wantText: "just a normal reply",
		},
		{
			name:     "text mentioning sticker word is untouched",
			input:    "我给你发一张 sticker 吧",
			wantText: "我给你发一张 sticker 吧",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripLegacyStickerTag(tt.input)
			if got != tt.wantText {
				t.Fatalf("text = %q, want %q", got, tt.wantText)
			}
		})
	}
}
