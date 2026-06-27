// PicoClaw - Ultra-lightweight personal AI agent

package agent

import (
	"context"
	"strings"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/utils"
)

func (al *AgentLoop) transcribeAudioInMessage(ctx context.Context, msg bus.InboundMessage) (bus.InboundMessage, bool) {
	if al.mediaStore == nil || len(msg.Media) == 0 {
		return msg, false
	}

	// If no transcriber is configured, still process audio annotations.
	// Audio will be passed as data URLs for multimodal models to handle directly.
	if al.transcriber == nil {
		hasAudio := false
		for _, ref := range msg.Media {
			if _, meta, err := al.mediaStore.ResolveWithMeta(ref); err == nil {
				if utils.IsAudioFile(meta.Filename, meta.ContentType) {
					hasAudio = true
					break
				}
			}
		}
		if !hasAudio {
			return msg, false
		}
		// Replace [voice] placeholders with a note that audio is attached.
		newContent := audioAnnotationRe.ReplaceAllString(msg.Content, "[audio message attached]")
		msg.Content = newContent
		return msg, true
	}

	// Transcribe each audio media ref in order.
	var transcriptions []string
	var keptMedia []string
	for _, ref := range msg.Media {
		path, meta, err := al.mediaStore.ResolveWithMeta(ref)
		if err != nil {
			logger.WarnCF("voice", "Failed to resolve media ref", map[string]any{"ref": ref, "error": err})
			keptMedia = append(keptMedia, ref)
			continue
		}
		if !utils.IsAudioFile(meta.Filename, meta.ContentType) {
			keptMedia = append(keptMedia, ref)
			continue
		}
		result, err := al.transcriber.Transcribe(ctx, path)
		if err != nil {
			logger.WarnCF("voice", "Transcription failed", map[string]any{"ref": ref, "error": err})
			transcriptions = append(transcriptions, "")
			keptMedia = append(keptMedia, ref)
			continue
		}
		transcriptions = append(transcriptions, result.Text)
	}

	if len(transcriptions) == 0 {
		return msg, false
	}

	al.sendTranscriptionFeedback(ctx, msg.Channel, msg.ChatID, msg.MessageID, transcriptions)

	// Replace audio annotations sequentially with transcriptions.
	idx := 0
	newContent := audioAnnotationRe.ReplaceAllStringFunc(msg.Content, func(match string) string {
		if idx >= len(transcriptions) {
			return match
		}
		text := transcriptions[idx]
		idx++
		if text == "" {
			return match
		}
		return "[voice: " + text + "]"
	})

	// Append any remaining transcriptions not matched by an annotation.
	for ; idx < len(transcriptions); idx++ {
		if transcriptions[idx] != "" {
			newContent += "\n[voice: " + transcriptions[idx] + "]"
		}
	}

	msg.Content = newContent
	msg.Media = keptMedia
	return msg, true
}

func (al *AgentLoop) sendTranscriptionFeedback(
	ctx context.Context,
	channel, chatID, messageID string,
	validTexts []string,
) {
	if !al.cfg.Voice.EchoTranscription {
		return
	}
	if al.channelManager == nil {
		return
	}

	var nonEmpty []string
	for _, t := range validTexts {
		if t != "" {
			nonEmpty = append(nonEmpty, t)
		}
	}

	var feedbackMsg string
	if len(nonEmpty) > 0 {
		feedbackMsg = "Transcript: " + strings.Join(nonEmpty, "\n")
	} else {
		feedbackMsg = "No voice detected in the audio"
	}

	err := al.channelManager.SendMessage(ctx, bus.OutboundMessage{
		Context:          bus.NewOutboundContext(channel, chatID, messageID),
		Content:          feedbackMsg,
		ReplyToMessageID: messageID,
	})
	if err != nil {
		logger.WarnCF("voice", "Failed to send transcription feedback", map[string]any{"error": err.Error()})
	}
}
