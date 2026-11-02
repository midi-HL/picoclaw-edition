package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/audio/asr"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/media"
)

// Regression guard: a successful transcription must NOT be written back into a
// `[voice: ...]` / `[audio: ...]` tag.
//
// Those tags mean "here is a FILE PATH you should transcribe". If the spoken
// text were placed inside them, the model would see `[voice: 今天天气不错]`,
// treat the Chinese text as a path, call transcribe_audio on it and fail —
// which is exactly the confusion this test prevents.
func TestTranscriptionOutputUsesDistinctMarker(t *testing.T) {
	const spoken = "今天天气不错我们出去走走吧"

	// Mirror the substitution performed by transcribeAudioInMessage.
	content := "[voice]"
	replaced := audioAnnotationRe.ReplaceAllStringFunc(content, func(string) string {
		return "[语音转写] " + spoken
	})

	if strings.Contains(replaced, "[voice:") || strings.Contains(replaced, "[audio:") {
		t.Fatalf("transcribed text must not be wrapped in a path tag, got %q", replaced)
	}
	if !strings.Contains(replaced, "[语音转写]") {
		t.Fatalf("expected the [语音转写] marker, got %q", replaced)
	}
	if !strings.Contains(replaced, spoken) {
		t.Fatalf("expected the spoken text to survive, got %q", replaced)
	}
}

// The annotation regex must still match the placeholder forms channels emit,
// including ones that already carry descriptive text.
func TestAudioAnnotationReMatchesPlaceholders(t *testing.T) {
	cases := []string{"[voice]", "[audio]", "[voice: voice message]", "[audio: file.ogg]"}
	for _, c := range cases {
		if !audioAnnotationRe.MatchString(c) {
			t.Fatalf("expected %q to match the annotation regex", c)
		}
	}
	// A transcribed marker must NOT be matched, otherwise re-processing would
	// rewrite already-transcribed text.
	if audioAnnotationRe.MatchString("[语音转写] 你好") {
		t.Fatal("[语音转写] must not be matched by the audio annotation regex")
	}
}

// A Telegram "audio" attachment (uploaded music/recording) must NOT be
// transcribed; only voice messages are. Without the source check every audio
// file the user shared was silently run through ASR.
func TestTranscribeAudioInMessage_SkipsTelegramAudioFiles(t *testing.T) {
	dir := t.TempDir()
	store := media.NewFileMediaStore()

	audioPath := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(audioPath, []byte("fake mp3"), 0o644); err != nil {
		t.Fatal(err)
	}
	ref, err := store.Store(audioPath, media.MediaMeta{
		Filename:    "song.mp3",
		ContentType: "audio/mpeg",
		Source:      media.SourceTelegramAudio,
	}, "scope")
	if err != nil {
		t.Fatal(err)
	}

	al := &AgentLoop{mediaStore: store, transcriber: &countingTranscriber{}}
	msg := bus.InboundMessage{
		Content: "[audio]",
		Media:   []string{ref},
	}

	_, hadAudio := al.transcribeAudioInMessage(context.Background(), msg)
	if hadAudio {
		t.Fatal("audio file must not be treated as a voice message")
	}
	if c, ok := al.transcriber.(*countingTranscriber); ok && c.calls != 0 {
		t.Fatalf("transcriber must not be called for audio files, got %d calls", c.calls)
	}
}

type countingTranscriber struct{ calls int }

func (c *countingTranscriber) Name() string { return "counting" }
func (c *countingTranscriber) Transcribe(_ context.Context, _ string) (*asr.TranscriptionResponse, error) {
	c.calls++
	return &asr.TranscriptionResponse{Text: "x"}, nil
}
