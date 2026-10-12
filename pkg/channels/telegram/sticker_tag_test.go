package telegram

import "testing"

func TestExtractStickerTag(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantText  string
		wantID    string
	}{
		{
			name:     "simple id",
			input:    "好的，给你一个 [SEND_STICKER: happy_dog]",
			wantText: "好的，给你一个 ",
			wantID:   "happy_dog",
		},
		{
			name:     "id with multiple underscores and digits",
			input:    "[SEND_STICKER: myadestes_1_nacho_5]",
			wantText: "",
			wantID:   "myadestes_1_nacho_5",
		},
		{
			name:     "markdown-stripped tag (underscores lost)",
			input:    "发出去了没？[SENDSTICKER: myadestes1nacho5]",
			wantText: "发出去了没？",
			wantID:   "myadestes1nacho5",
		},
		{
			name:     "with surrounding spaces",
			input:    "hi [ SEND_STICKER :  abc_def ] bye",
			wantText: "hi  bye",
			wantID:   "abc_def",
		},
		{
			name:     "no tag",
			input:    "just a normal reply",
			wantText: "just a normal reply",
			wantID:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, id := extractStickerTag(tt.input)
			if id != tt.wantID {
				t.Fatalf("id = %q, want %q", id, tt.wantID)
			}
			if text != tt.wantText {
				t.Fatalf("text = %q, want %q", text, tt.wantText)
			}
		})
	}
}
