// PicoClaw - Ultra-lightweight personal AI agent

package agent

import (
	"context"
	"strings"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/media"
	"github.com/sipeed/picoclaw/pkg/utils"
)

func (al *AgentLoop) transcribeAudioInMessage(ctx context.Context, msg bus.InboundMessage) (bus.InboundMessage, bool) {
	if al.mediaStore == nil || len(msg.Media) == 0 {
		return msg, false
	}

	// No transcriber configured. Do NOT forward raw audio to the model (text
	// models reject it with HTTP 400 and the turn dies). Keep the media ref so
	// resolveMediaRefs emits a `[voice:/path]` path tag instead of inlined bytes,
	// and add a note telling the model it can call transcribe_audio — which will
	// then report precisely why it is unavailable.
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
		const notice = "[系统提示：用户发送了一条语音消息。当前没有可用的语音转文字服务，" +
			"你无法直接听到音频。如果消息里有 `[voice:/路径]` 或 `[audio:/路径]` 标签，" +
			"可以调用 transcribe_audio 工具尝试转写（它会返回失败原因）；" +
			"也可以直接告诉用户可以在「配置 → 语音转文字」里开启本地或云端转写，" +
			"或把内容打成文字发过来。不要假装听到了内容。]"
		if strings.TrimSpace(msg.Content) == "" {
			msg.Content = notice
		} else if !strings.Contains(msg.Content, notice) {
			msg.Content += "\n" + notice
		}
		logger.WarnCF("voice", "No transcriber: kept audio ref for transcribe_audio tool", map[string]any{
			"channel": msg.Channel, "chat_id": msg.ChatID,
		})
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
		// Only *voice messages* are transcribed automatically.
		//
		// A Telegram "audio" attachment is an uploaded music track / recording,
		// not something the user said to the assistant. Transcribing it wastes
		// time and surprises the user. Media without an explicit source keeps the
		// old behaviour so other channels are unaffected.
		if meta.Source == media.SourceTelegramAudio {
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
			return "[系统提示：用户发送了一条语音，但语音转写失败（可能是 ASR 服务未就绪或音频格式不支持）。请如实告知用户转写失败，并建议重发或改用文字。]"
		}
		// IMPORTANT: this is TRANSCRIBED TEXT, not a file path. Use a distinct
		// marker so the model never mistakes it for `[voice:/path]` and tries to
		// call transcribe_audio on spoken words.
		return "[语音转写] " + text
	})

	// Append any remaining transcriptions not matched by an annotation.
	for ; idx < len(transcriptions); idx++ {
		if transcriptions[idx] != "" {
			newContent += "\n[语音转写] " + transcriptions[idx]
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
