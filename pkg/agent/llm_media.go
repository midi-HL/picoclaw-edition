package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/utils"
)

var resolvedImagePathTagRegex = regexp.MustCompile(`\[image:[^\s\]][^\]]*\]`)

func messagesContainMedia(messages []providers.Message) bool {
	for _, msg := range messages {
		for _, ref := range msg.Media {
			if strings.TrimSpace(ref) != "" {
				return true
			}
		}
	}
	return false
}

func stripMessageMedia(messages []providers.Message) []providers.Message {
	if !messagesContainMedia(messages) {
		return messages
	}
	stripped := make([]providers.Message, len(messages))
	for i, msg := range messages {
		stripped[i] = msg
		stripped[i].Media = nil
	}
	return stripped
}

func isVisionUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())

	// OpenRouter (and OpenAI-compatible) style.
	if strings.Contains(msg, "no endpoints found that support image input") {
		return true
	}

	// Common provider variants.
	if strings.Contains(msg, "does not support image input") ||
		strings.Contains(msg, "does not support image inputs") ||
		strings.Contains(msg, "does not support images") ||
		strings.Contains(msg, "image input is not supported") ||
		strings.Contains(msg, "images are not supported") ||
		strings.Contains(msg, "does not support vision") ||
		strings.Contains(msg, "unsupported content type: image_url") {
		return true
	}

	// Some providers return a generic "invalid" message that still mentions image_url.
	if strings.Contains(msg, "image_url") && strings.Contains(msg, "invalid") {
		return true
	}

	// DeepSeek and other strict providers reject the image_url field at the
	// JSON schema level with an "unknown variant" error rather than a semantic
	// "not supported" message.
	if strings.Contains(msg, "unknown variant") && strings.Contains(msg, "image_url") {
		return true
	}

	// Audio / video input rejection. Non-omni models reject these fields the same
	// way they reject images, so the same fallback (strip media, retry as text)
	// applies. Match both semantic messages and schema-level "unknown variant".
	if strings.Contains(msg, "input_audio") || strings.Contains(msg, "audio input") ||
		strings.Contains(msg, "does not support audio") || strings.Contains(msg, "audio is not supported") ||
		strings.Contains(msg, "unsupported audio") {
		return true
	}
	if strings.Contains(msg, "video_url") || strings.Contains(msg, "video input") ||
		strings.Contains(msg, "does not support video") || strings.Contains(msg, "video is not supported") ||
		strings.Contains(msg, "unsupported video") {
		return true
	}
	if strings.Contains(msg, "unknown variant") &&
		(strings.Contains(msg, "audio") || strings.Contains(msg, "video")) {
		return true
	}

	return false
}

func visionUnsupportedModelError(modelName string, imageModelConfigured bool) error {
	modelName = strings.TrimSpace(modelName)
	if imageModelConfigured {
		if modelName != "" {
			return fmt.Errorf(
				"selected vision model %q does not support image input; update agents.defaults.image_model to a multimodal model",
				modelName,
			)
		}
		return fmt.Errorf(
			"selected vision model does not support image input; update agents.defaults.image_model to a multimodal model",
		)
	}
	if modelName != "" {
		return fmt.Errorf(
			"active model %q does not support image input; configure agents.defaults.image_model with a multimodal model",
			modelName,
		)
	}
	return fmt.Errorf(
		"the active model does not support image input; configure agents.defaults.image_model with a multimodal model",
	)
}

func sameCandidateSet(a, b []providers.FallbackCandidate) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].StableKey() != b[i].StableKey() {
			return false
		}
	}
	return true
}

func messagesContainCurrentTurnMediaTurn(messages []providers.Message) bool {
	for _, msg := range messages {
		if len(msg.Media) > 0 {
			return true
		}
		if resolvedImagePathTagRegex.MatchString(msg.Content) {
			return true
		}
	}
	return false
}

// messagesContainOnlyAudioVideo reports whether the media attached to the given
// messages consists solely of audio/video (no images). Used to decide whether an
// "unsupported media" error can be recovered by dropping the attachments: audio
// and video usually arrive passively from channels, so failing the whole turn
// would leave the user with nothing.
func messagesContainOnlyAudioVideo(messages []providers.Message) bool {
	sawAudioVideo := false
	for _, msg := range messages {
		for _, ref := range msg.Media {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			switch {
			case strings.HasPrefix(ref, "data:audio/"), strings.HasPrefix(ref, "data:video/"):
				sawAudioVideo = true
			default:
				// Anything else (image data URL, media:// ref) means we cannot
				// assume audio/video-only.
				return false
			}
		}
		// Images are exposed to the model as path tags in message content rather
		// than media entries, so also treat an [image:...] tag as "has image".
		if imagePlaceholderRegex.MatchString(msg.Content) || resolvedImagePathTagRegex.MatchString(msg.Content) {
			return false
		}
	}
	return sawAudioVideo
}

// messagesContainVideo checks if any message in the current turn contains video data URLs.
func messagesContainVideo(messages []providers.Message) bool {
	for _, msg := range messages {
		for _, ref := range msg.Media {
			if strings.HasPrefix(ref, "data:video/") {
				return true
			}
		}
	}
	return false
}

// describeVideoProxy sends video to the video_model with a description prompt,
// then replaces the video data URLs in the messages with the description.
// This implements the "delegation" pattern: video_model describes → main model uses description.
func (p *Pipeline) describeVideoProxy(ts *turnState, exec *turnExecution) error {
	if p == nil || ts == nil || ts.agent == nil || exec == nil {
		return nil
	}
	videoModel := strings.TrimSpace(p.Cfg.Agents.Defaults.VideoModel)
	if videoModel == "" || len(ts.agent.VideoCandidates) == 0 {
		return nil
	}

	turnMsgs := currentTurnMessages(exec.callMessages, exec.currentTurnStart)
	if !messagesContainVideo(turnMsgs) {
		return nil
	}

	// Get the video model provider
	videoProvider := ts.agent.Provider
	firstCandidate := ts.agent.VideoCandidates[0]
	if provider, err := providerForFallbackCandidate(
		ts.agent, ts.agent.Provider, ts.agent.VideoCandidates,
		firstCandidate,
	); err == nil && provider != nil {
		videoProvider = provider
	}

	resolvedModel := resolvedCandidateModel(ts.agent.VideoCandidates, videoModel)

	// Process each message with video
	for i := range exec.callMessages {
		msg := &exec.callMessages[i]
		if len(msg.Media) == 0 {
			continue
		}

		var videoDataURLs []string
		var remainingMedia []string
		for _, ref := range msg.Media {
			if strings.HasPrefix(ref, "data:video/") {
				videoDataURLs = append(videoDataURLs, ref)
			} else {
				remainingMedia = append(remainingMedia, ref)
			}
		}

		if len(videoDataURLs) == 0 {
			continue
		}

		// Describe each video
		for _, videoURL := range videoDataURLs {
			describeMsg := providers.Message{
				Role:    "user",
				Content: "Describe this video in detail. Include: what is happening, key objects, people, actions, scene setting, and any notable details. Be specific and factual.",
				Media:   []string{videoURL},
			}

			resp, err := videoProvider.Chat(
				context.Background(),
				[]providers.Message{describeMsg},
				nil,
				resolvedModel,
				nil,
			)
			if err != nil {
				logger.WarnCF("agent", "Video description proxy failed", map[string]any{
					"model": videoModel,
					"error": err.Error(),
				})
				continue
			}

			description := strings.TrimSpace(resp.Content)
			if description == "" {
				continue
			}

			// Inject description into the message content
			msg.Content = msg.Content + "\n\n[系统消息：以下是用户发送视频的描述]\n" + description
			logger.InfoCF("agent", "Video described by proxy model", map[string]any{
				"model":       videoModel,
				"description": utils.Truncate(description, 100),
			})
		}

		// Remove video from media (already described)
		msg.Media = remainingMedia
	}

	return nil
}

func (p *Pipeline) routeMediaTurn(ts *turnState, exec *turnExecution) error {
	if p == nil || ts == nil || ts.agent == nil || exec == nil ||
		!messagesContainCurrentTurnMediaTurn(currentTurnMessages(exec.callMessages, exec.currentTurnStart)) {
		return nil
	}

	// First, try video proxy (describe video with video_model, then pass to main model)
	if err := p.describeVideoProxy(ts, exec); err != nil {
		return err
	}

	var targetCandidates []providers.FallbackCandidate
	var targetModelName string
	var routeReason string

	switch {
	case len(ts.agent.ImageCandidates) > 0:
		targetCandidates = append([]providers.FallbackCandidate(nil), ts.agent.ImageCandidates...)
		targetModelName = strings.TrimSpace(p.Cfg.Agents.Defaults.ImageModel)
		routeReason = "configured_image_model"
	case exec.usedLight && len(ts.agent.Candidates) > 0:
		targetCandidates = append([]providers.FallbackCandidate(nil), ts.agent.Candidates...)
		targetModelName = strings.TrimSpace(ts.agent.Model)
		routeReason = "bypass_light_model_for_media"
	default:
		return nil
	}

	if len(targetCandidates) == 0 {
		return nil
	}

	targetModel := resolvedCandidateModel(targetCandidates, targetModelName)
	targetProvider := exec.activeProvider
	firstCandidate := targetCandidates[0]
	if provider, err := providerForFallbackCandidate(
		ts.agent,
		ts.agent.Provider,
		ts.agent.Candidates,
		firstCandidate,
	); err != nil {
		return err
	} else if provider != nil {
		targetProvider = provider
	}

	resolvedModelName := resolvedCandidateModelName(targetCandidates, targetModelName)
	if sameCandidateSet(exec.activeCandidates, targetCandidates) &&
		exec.activeModel == targetModel &&
		exec.llmModelName == resolvedModelName {
		return nil
	}

	exec.activeCandidates = targetCandidates
	exec.activeModel = targetModel
	exec.activeProvider = targetProvider
	exec.activeModelConfig = resolveActiveModelConfig(
		p.Cfg,
		ts.agent.Workspace,
		targetCandidates,
		targetModel,
		p.Cfg.Agents.Defaults.Provider,
	)
	exec.llmModelName = resolvedModelName
	exec.usedLight = false

	logger.InfoCF("agent", "Media turn routing selected model", map[string]any{
		"agent_id":       ts.agent.ID,
		"reason":         routeReason,
		"model":          exec.activeModel,
		"model_name":     exec.llmModelName,
		"candidates":     len(exec.activeCandidates),
		"messages_count": len(exec.callMessages),
	})

	return nil
}
