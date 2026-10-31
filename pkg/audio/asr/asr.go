package asr

import (
	"context"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
)

const elevenLabsSupportedModelID = "scribe_v1"

func ElevenLabsSupportedModelID() string {
	return elevenLabsSupportedModelID
}

// LocalFactory builds a local (sherpa-onnx) transcriber from config. It is
// registered by the local package to avoid an import cycle (local imports asr
// for the Transcriber interface).
type LocalFactory func(cfg *config.LocalASRConfig) (Transcriber, error)

var localFactory LocalFactory

// RegisterLocalFactory wires the sherpa-onnx implementation into detection.
func RegisterLocalFactory(f LocalFactory) { localFactory = f }

type Transcriber interface {
	Name() string
	Transcribe(ctx context.Context, audioFilePath string) (*TranscriptionResponse, error)
}

type TranscriptionResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

func supportsAudioTranscription(modelCfg *config.ModelConfig) bool {
	protocol, _ := providers.ExtractProtocol(modelCfg)

	switch protocol {
	case "openai", "azure",
		"litellm", "openrouter", "groq", "zhipu", "gemini", "nvidia",
		"ollama", "moonshot", "shengsuanyun", "deepseek", "cerebras",
		"vivgrid", "volcengine", "vllm", "qwen-portal", "qwen-intl", "qwen-us",
		"mistral", "avian", "minimax", "longcat", "modelscope", "novita",
		"alibaba-coding", "zai":
		// These protocols all go through the OpenAI-compatible or Azure provider path in
		// providers.CreateProviderFromConfig, so they are the only ones that can supply
		// the audio media payload shape expected by NewAudioModelTranscriber.

		// TODO: Further restrict this by modelID, since not every model under these
		// protocols supports audio transcription.
		return true
	default:
		return false
	}
}

func supportsWhisperTranscription(modelCfg *config.ModelConfig) bool {
	protocol, _ := providers.ExtractProtocol(modelCfg)

	switch protocol {
	case "openai", "litellm", "openrouter", "groq", "zhipu", "gemini", "nvidia",
		"ollama", "moonshot", "shengsuanyun", "deepseek", "cerebras",
		"vivgrid", "volcengine", "vllm", "qwen-portal", "qwen-intl", "qwen-us",
		"mistral", "avian", "minimax", "longcat", "modelscope", "novita",
		"alibaba-coding", "zai", "mimo":
		return true
	default:
		return false
	}
}

func whisperModelID(modelCfg *config.ModelConfig) string {
	if modelCfg == nil || modelCfg.APIKey() == "" {
		return ""
	}

	if !supportsWhisperTranscription(modelCfg) {
		return ""
	}

	_, modelID := providers.ExtractProtocol(modelCfg)
	if strings.Contains(strings.ToLower(modelID), "whisper") {
		return modelID
	}
	return ""
}

func isElevenLabsTranscriptionModel(modelCfg *config.ModelConfig) bool {
	if modelCfg == nil || modelCfg.APIKey() == "" {
		return false
	}

	protocol, _ := providers.ExtractProtocol(modelCfg)
	return protocol == "elevenlabs"
}

func transcriberFromModelConfig(modelCfg *config.ModelConfig) Transcriber {
	if modelCfg == nil {
		return nil
	}

	if isElevenLabsTranscriptionModel(modelCfg) {
		_, modelID := providers.ExtractProtocol(modelCfg)
		return NewElevenLabsTranscriber(modelCfg.APIKey(), modelCfg.APIBase, modelID)
	}
	if modelID := whisperModelID(modelCfg); modelID != "" {
		return NewWhisperTranscriber(modelCfg)
	}
	if supportsAudioTranscription(modelCfg) {
		return NewAudioModelTranscriber(modelCfg)
	}
	return nil
}

func fallbackTranscriberFromModelConfig(modelCfg *config.ModelConfig) Transcriber {
	if modelCfg == nil {
		return nil
	}

	if isElevenLabsTranscriptionModel(modelCfg) {
		_, modelID := providers.ExtractProtocol(modelCfg)
		return NewElevenLabsTranscriber(modelCfg.APIKey(), modelCfg.APIBase, modelID)
	}
	if modelID := whisperModelID(modelCfg); modelID != "" {
		return NewWhisperTranscriber(modelCfg)
	}
	return nil
}

// DetectTranscriber inspects cfg and returns the appropriate Transcriber, or
// nil if no supported transcription provider is configured.
func DetectTranscriber(cfg *config.Config) Transcriber {
	if cfg == nil {
		return nil
	}

	// Local (sherpa-onnx) takes precedence when explicitly enabled.
	if cfg.Voice.IsLocalASRActive() && localFactory != nil {
		if tr, err := localFactory(cfg.Voice.LocalASR); err == nil && tr != nil {
			return tr
		} else if err != nil {
			logger.WarnCF("voice", "Local ASR unavailable, falling back to cloud", map[string]any{
				"error": err.Error(),
			})
		}
	}

	if modelName := strings.TrimSpace(cfg.Voice.ModelName); modelName != "" {
		modelCfg, err := cfg.GetModelConfig(modelName)
		if err == nil {
			if tr := transcriberFromModelConfig(modelCfg); tr != nil {
				return tr
			}
		}
	}

	// Fall back to compatibility scanning for legacy auto-detected ASR providers.
	for _, mc := range cfg.ModelList {
		if tr := fallbackTranscriberFromModelConfig(mc); tr != nil {
			return tr
		}
	}
	return nil
}
