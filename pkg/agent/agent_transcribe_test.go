package agent

import (
	"strings"
	"testing"
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
