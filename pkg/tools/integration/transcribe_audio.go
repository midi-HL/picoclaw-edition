package integrationtools

import (
	"context"
	"fmt"
	"strings"
)

// AudioTranscriber transcribes an audio file to text. The agent wires this to
// whichever ASR backend is configured (local sherpa-onnx or a cloud model).
//
// The returned error is shown to the model verbatim, so implementations should
// produce a message that tells the model what to do next (install ffmpeg,
// enable the feature, retry, …) rather than a bare stack trace.
type AudioTranscriber func(ctx context.Context, audioPath string) (string, error)

// TranscribeAudioTool lets the model transcribe an audio file on demand.
//
// Why a tool instead of automatic transcription:
//   - Not every model accepts audio, and forwarding raw audio to a text model
//     makes the provider reject the whole request (HTTP 400 "unsupported part"),
//     which kills the turn before the model can even apologise.
//   - A tool keeps the decision with the model: it sees "[voice: /path]" in the
//     message, can call this tool, and gets either the text or a precise reason
//     why it failed — then answers the user accordingly.
type TranscribeAudioTool struct {
	transcriber AudioTranscriber
	// statusFn reports whether ASR is configured, for a precise error message.
	statusFn func(ctx context.Context) error
}

func NewTranscribeAudioTool() *TranscribeAudioTool {
	return &TranscribeAudioTool{}
}

func (t *TranscribeAudioTool) Name() string { return "transcribe_audio" }

func (t *TranscribeAudioTool) Description() string {
	return "Transcribe a local audio file to text (speech-to-text). " +
		"Use this when a user message contains an audio/voice attachment, e.g. a tag like " +
		"`[voice:/path/to/file.oga]` or `[audio:/path/file.wav]`. " +
		"Pass the path exactly as it appears in the message. " +
		"The result is the spoken text; tell the user you understood it from speech, " +
		"not from typed text. " +
		"If this returns an error, report it honestly and suggest what the user can do " +
		"(enable speech-to-text, fix its configuration, or send text instead)."
}

func (t *TranscribeAudioTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the audio file, exactly as shown in the message tag.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *TranscribeAudioTool) SetTranscriber(fn AudioTranscriber) { t.transcriber = fn }
func (t *TranscribeAudioTool) SetStatusCheck(fn func(context.Context) error) {
	t.statusFn = fn
}

func (t *TranscribeAudioTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	path, _ := args["path"].(string)
	path = strings.TrimSpace(path)
	if path == "" {
		return &ToolResult{
			ForLLM:  "path is required (the audio file path from the message tag).",
			IsError: true,
		}
	}

	// Report "not enabled" distinctly from "enabled but broken": the model
	// should ask the user to enable the feature rather than to debug it.
	if t.statusFn != nil {
		if err := t.statusFn(ctx); err != nil {
			return &ToolResult{
				ForLLM: fmt.Sprintf(
					"Speech-to-text is not available: %v. "+
						"Tell the user this feature is not enabled (or not configured correctly) "+
						"and offer to help them set it up in Settings → Speech-to-text; "+
						"also suggest sending the content as text for now.",
					err),
				IsError: true,
				Err:     err,
			}
		}
	}

	if t.transcriber == nil {
		return &ToolResult{
			ForLLM: "Speech-to-text is not available in this deployment. " +
				"Tell the user the feature is not enabled and suggest sending text instead.",
			IsError: true,
		}
	}

	text, err := t.transcriber(ctx, path)
	if err != nil {
		return &ToolResult{
			ForLLM: fmt.Sprintf(
				"Speech-to-text failed for %q: %v. "+
					"Tell the user honestly that the audio could not be transcribed, "+
					"give the reason above, and ask whether they want help fixing the "+
					"configuration or prefer to send text instead.",
				path, err),
			IsError: true,
			Err:     err,
		}
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return &ToolResult{
			ForLLM: "The audio was transcribed but no speech was detected (silence or noise). " +
				"Tell the user no speech was found and ask them to re-send.",
			IsError: true,
		}
	}

	// Make the provenance explicit so the model does not treat this as typed text.
	return &ToolResult{
		ForLLM: "[语音转写结果 / speech-to-text] " + text,
		Silent: true,
	}
}
