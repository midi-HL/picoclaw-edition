[中文版](docs/project/README.zh.md) | [日本語](docs/project/README.ja.md) | [한국어](docs/project/README.ko.md) | [Português](docs/project/README.pt-br.md) | [Tiếng Việt](docs/project/README.vi.md) | [Français](docs/project/README.fr.md) | [Italiano](docs/project/README.it.md) | [Bahasa Indonesia](docs/project/README.id.md) | [Malay](docs/project/README.ms.md) | **English**

# PicoClaw 32-bit Fork — Modifications & Documentation

This is the primary documentation for this fork (`picoclaw-edition`). It describes all modifications made compared to the upstream [PicoClaw](https://github.com/sipeed/picoclaw) project, including 32-bit platform support, multimodal API changes, and new tools.

---

## Table of Contents

- [1. MiMo Multimodal Support](#1-mimo-multimodal-support)
- [2. Video Understanding](#2-video-understanding)
- [3. Telegram Video Messages](#3-telegram-video-messages)
- [4. load_video Tool](#4-load_video-tool)
- [5. Audio Data URL Encoding](#5-audio-data-url-encoding)
- [6. Config Robustness](#6-config-robustness)
- [7. API Changes](#7-api-changes)
- [8. 32-bit Platform Support](#8-32-bit-platform-support)
- [9. Telegram Stickers (Custom Emoji Packs)](#9-telegram-stickers-custom-emoji-packs)
- [10. Security](#10-security)
- [11. Group Chat Attribution & Media Fallback](#11-group-chat-attribution--media-fallback)
- [12. API Key & Provider Rotation](#12-api-key--provider-rotation)
- [13. Web Chat File Upload](#13-web-chat-file-upload)
- [Known Limitations](#known-limitations)

---

## 1. Dual Multimodal API Format Support (Standard OpenAI + MiMo)

**Problem:** The upstream provider code sends audio in standard OpenAI format (`{"data": "base64", "format": "wav"}`), but MiMo's API expects a full data URL (`{"data": "data:audio/wav;base64,..."}`). A single format cannot satisfy both providers.

**Solution:** Added provider-aware format selection that automatically detects the target provider and sends the appropriate format.

### Implementation

**New types and functions in `pkg/providers/common/common.go`:**

```go
// SerializeOptions carries provider identity for format-specific serialization.
type SerializeOptions struct {
    ProviderName string  // e.g. "mimo", "openai", "anthropic"
    APIBase      string  // e.g. "https://api.xiaomimimo.com/v1"
}

// Provider-aware serialization — picks format based on target provider.
func SerializeMessagesWithOptions(messages []Message, opts *SerializeOptions) []any

// Backward-compatible — uses standard OpenAI format by default.
func SerializeMessages(messages []Message) []any
```

**Provider detection logic:**
```go
func isMiMoProvider(opts *SerializeOptions) bool {
    return opts.ProviderName == "mimo" ||
           strings.Contains(opts.APIBase, "xiaomimimo.com")
}
```

**Call chain:**
```
openai_compat.Provider.buildRequestBody()
  → passes p.providerName + p.apiBase as SerializeOptions
  → SerializeMessagesWithOptions selects format per provider
```

### Format Selection by Provider

| Media Type | MiMo Provider | Standard OpenAI Provider |
|-----------|--------------|------------------------|
| **Image** | `{"type": "image_url", "image_url": {"url": "data:image/..."}}` | Same (universal format) |
| **Audio** | `{"type": "input_audio", "input_audio": {"data": "data:audio/wav;base64,..."}}` | `{"type": "input_audio", "input_audio": {"data": "base64...", "format": "wav"}}` |
| **Video** | `{"type": "video_url", "video_url": {"url": "data:video/mp4;base64,..."}, "fps": 2, "media_resolution": "default"}` | Skipped (no standard type) |

### Key Differences Explained

**Audio format split:**
- MiMo expects the **full data URL** (`data:audio/wav;base64,...`) in the `data` field, no `format` field
- Standard OpenAI expects **raw base64** in `data` and the MIME subtype in a separate `format` field
- The code parses the data URL with `ParseDataAudioURL()` to extract both parts when needed

**Video format:**
- `video_url` is a MiMo-specific type with `fps` and `media_resolution` parameters
- There is no standard OpenAI video type — video is silently skipped for non-MiMo providers
- Fallback: the `video_model` delegation proxy can describe video content for non-video-capable models

### Backward Compatibility

- `SerializeMessages()` (no options) → standard OpenAI format, same as upstream
- `SerializeMessagesWithOptions(msgs, nil)` → standard OpenAI format
- `SerializeMessagesWithOptions(msgs, &SerializeOptions{ProviderName: "mimo"})` → MiMo format

---

## 2. Video Understanding

**Problem:** The `video_model` configuration field existed but was never used by the agent code.

**Changes:**
- `pkg/agent/instance.go` — Added `VideoCandidates` field to `AgentInstance` and resolved `video_model` candidates at startup.
- `pkg/agent/llm_media.go` — Added `describeVideoProxy()` function implementing a **delegation pattern**:
  1. Detects `data:video/` URLs in the current turn
  2. Sends video + description prompt to `video_model`
  3. Injects the description into the message content as `[系统消息：以下是用户发送视频的描述]`
  4. Main model receives the description and replies normally
- `pkg/agent/llm_media.go` — `routeMediaTurn` calls `describeVideoProxy` before falling back to image model routing.

**Flow:**
```
User sends video
  → video_model describes video
  → Description injected into message
  → Main model replies using description as context
```

---

## 3. Telegram Video Messages

**Problem:** `collectTelegramMessageParts` handled Photo, Voice, Audio, and Document — but not Video. Video messages were silently dropped.

**Changes:**
- `pkg/channels/telegram/telegram.go` — Added `msg.Video` handling: downloads video file, stores in media store, adds `[video]` tag to message content.

---

## 4. load_video Tool

**New feature:** A tool that allows the AI to load and analyze local video files.

**Files:**
- `pkg/tools/fs/load_video.go` — New tool implementation (mirrors `load_image`).
- `pkg/tools/fs_facade.go` — Added `LoadVideoTool` type alias and `NewLoadVideoTool` factory.
- `pkg/agent/agent_init.go` — Registered `load_video` tool.
- `pkg/config/config.go` — Added `LoadVideo ToolConfig` field.
- `pkg/agent/context.go` — Updated multimodal system prompt to mention `load_video`.

**How it works:**
1. AI calls `load_video(path="video.mp4")`
2. Tool validates path, detects MIME type, stores in media store
3. Returns `media://` reference
4. `resolveMediaRefs` encodes as `data:video/mp4;base64,...`
5. Provider sends as `video_url` format to MiMo

---

## 5. Audio Data URL Encoding

**Problem:** Audio from user messages was not encoded as data URLs for the model.

**Changes:**
- `pkg/agent/agent_media.go` — `resolveMediaRefs` now encodes audio and video as data URLs for both user messages and tool results.
- `pkg/agent/prompt_turn.go` — `toolImageFollowUpPromptMessage` detects video data URLs and updates the synthetic user message text accordingly.

---

## 6. Config Robustness

### Unknown fields are warnings, not errors

**Problem:** Config files with deprecated fields (e.g., from older versions) caused startup failures.

**Changes:**
- `pkg/config/diagnostics.go` — `decodeJSONWithDiagnostics` now logs unknown fields as warnings to stderr instead of returning errors.

### Config API request body limit

**Problem:** The PATCH/PUT `/api/config` endpoint had a 1MB body limit, too small for voice clone audio data in base64.

**Changes:**
- `web/backend/api/config.go` — Increased body limit from 1MB to 20MB for both PUT and PATCH handlers.

### VoiceConfig MimoConfig field

**Problem:** The Go `VoiceConfig` struct had no `MimoConfig` field, so MiMo-specific settings were lost during JSON round-trip.

**Changes:**
- `pkg/config/config.go` — Added `VoiceMimoConfig` struct with ASR fields (`asr_provider`, `asr_language`, `asr_api_key`).

---

## 7. API Changes

This fork adds the following API capabilities. For detailed API documentation, see [API Reference](docs/api/README.md).

### Chat API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/chat` | POST | Synchronous chat — send message, receive complete reply |
| `/api/chat/stream` | POST | Streaming chat — SSE real-time token output |

### New Tool: load_video

The `load_video` tool is registered as a callable tool for the AI agent. It accepts a file path and returns a video analysis reference.

**Tool parameters:**
```json
{
  "path": "path/to/video.mp4"
}
```

**Tool result:**
```
Video loaded: video.mp4
[video: /path/to/video.mp4]
```

The video is automatically encoded as a data URL and sent to the model in `video_url` format.

---

## 8. 32-bit Platform Support

> This section documents the 32-bit platform support that was added to this fork.

### Supported 32-bit targets

| OS | GOARCH | Binary Name | Minimum OS Version |
|----|--------|-------------|-------------------|
| Linux | `386` | `picoclaw-linux-386` | Any 32-bit Linux with kernel 2.6+ |
| Linux | `arm` (GOARM=7) | `picoclaw-linux-arm` | ARMv7 Linux (e.g. Raspberry Pi) |
| Linux | `mipsle` | `picoclaw-linux-mipsle` | MIPS32 little-endian Linux (softfloat) |
| Linux | `mips` | `picoclaw-linux-mips` | MIPS32 big-endian Linux (softfloat) |
| Windows | `386` | `picoclaw-windows-386.exe` | Windows XP SP3 / Vista / 7 / 8 / 8.1 / 10 (32-bit) |

### What was changed

- Added `linux/386`, `linux/arm`, `linux/mipsle`, `linux/mips` build targets to the Makefile
- The `windows/386` target was already present in the Makefile and `.goreleaser.yaml`
- Source files using modernc sqlite/libc have build tags excluding both `mipsle` and `mips` big-endian

### Implementation details

- Uses the pure-Go olm cryptographic implementation via the `goolm` build tag (no CGO / `libolm` dependency)
- All Windows APIs used are Vista/Win7 level — no Win10+ exclusive APIs
- `unsafe.Pointer` usage is architecture-neutral
- Feishu/Lark channel is **not available** on 32-bit (upstream SDK limitation, handled gracefully at runtime)
- Matrix channel is **not available** on MIPS (both LE and BE) due to modernc sqlite/libc build constraints
- MIPS targets use `GOMIPS=softfloat` for compatibility with FP-lacking kernels

### Build from source

```bash
# Linux 32-bit x86
CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -v -tags goolm,stdjson -o build/picoclaw-linux-386 ./cmd/picoclaw

# Linux 32-bit ARM (GOARM=7)
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -v -tags goolm,stdjson -o build/picoclaw-linux-arm ./cmd/picoclaw

# Linux 32-bit MIPS LE (softfloat, no goolm)
CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -v -tags stdjson -o build/picoclaw-linux-mipsle ./cmd/picoclaw

# Linux 32-bit MIPS BE (softfloat, no goolm)
CGO_ENABLED=0 GOOS=linux GOARCH=mips GOMIPS=softfloat go build -v -tags stdjson -o build/picoclaw-linux-mips ./cmd/picoclaw

# Windows 32-bit (cross-compile from any OS)
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -v -tags goolm,stdjson -o build/picoclaw-windows-386.exe ./cmd/picoclaw

# Or use the Makefile targets (builds all platforms including 32-bit):
make build-all
```

### Runtime system requirements

| Resource | Minimum |
|----------|---------|
| CPU | Any x86 processor with SSE2 support |
| RAM | 512 MB |
| Disk | 100 MB (binary) + workspace storage |
| Network | Internet access for LLM API calls |

---

## 9. Telegram Stickers (Custom Emoji Packs)

This fork adds **bidirectional** Telegram sticker support.

### Inbound — user sends a sticker to the AI

When a user sends a Telegram sticker, the bot now forwards it to the model as a
real image so the AI can *see* and understand it.

**Problem:** User-sent images are normally turned into a path tag (`[image:/path]`)
by `resolveMediaRefs`, and the image bytes are **not** sent to the model — the
model is expected to call the `load_image` tool if it wants to look. Stickers were
therefore invisible to the model (it only saw a path), so it could not react to them.

**Fix:**
- `pkg/channels/telegram/telegram.go` — the sticker is downloaded (animated/video
  stickers are downgraded to their static `.jpg` thumbnail; regular stickers use
  `.webp`) and registered in the media store **tagged with a dedicated source**
  (`media.SourceTelegramSticker`). The message text carries an explicit system hint
  telling the model that this is a **Telegram sticker / custom emoji pack**, *not* a
  photo the user uploaded.
- `pkg/agent/agent_media.go` — `resolveMediaRefs` inlines **sticker** media (and
  user audio/video) as `data:...;base64,` URLs for the current turn, using the MIME
  detected from the file's magic bytes. Regular user images keep the upstream
  behaviour (path tag only + on-demand `load_image`), so large photos are never
  re-sent inline.
- Robustness — media refs are **persisted into session history**, so an inline
  base64 blob must never be stored there (it would be re-sent, and re-bloated, on
  every later turn). Stickers are stored as small refs and only inlined at send
  time; additionally, `resolveMediaRefs` drops any inline `data:` ref found in
  **historical** turns, which self-heals sessions polluted by earlier builds.

### Outbound — AI sends a sticker

The AI sends stickers through a dedicated **`send_sticker` tool**
(`pkg/tools/integration/sticker.go`, registered in `pkg/agent/agent_init.go`), not
by embedding text markers.

**Why a tool instead of the old `[SEND_STICKER: id]` text directive:** the marker
approach parsed the model's reply text inside the channel layer, which was fragile:
- outbound text is split into chunks *before* it reaches the channel, so a marker
  near the 4000-rune boundary could be cut in half and silently dropped;
- the model never learned whether the sticker was actually delivered, so it could
  claim success when nothing was sent;
- failures only appeared in server logs.

The tool returns an explicit success/error result to the model, so it always knows
the real outcome. Its description marks it **Telegram-only**.

Supporting behaviour:
- **Legacy markers are stripped, never acted on.** If an old conversation makes the
  model emit `[SEND_STICKER: id]`, `stripLegacyStickerTag` removes it from the
  visible text (both in `TelegramChannel.Send` and in the streaming finalizer) so
  users never see the raw directive. Delivery only ever happens via the tool.
- **Fuzzy ID fallback.** If an ID arrives with separators dropped, lookup matches
  IDs ignoring `_` and `-`.
- **file_id → local file fallback.** Telegram `file_id`s are bot-scoped; if sending
  by a cached `file_id` is rejected, the channel uploads the local file instead
  (and clears the cached ID only when the error says it is actually invalid).
- **Cross-machine path repair.** Sticker DB entries store absolute paths; if the DB
  was moved from another host, the file is re-resolved inside the current media
  directory by basename (applies to both sending and preview).

The available sticker list is injected into the system prompt by
`pkg/agent/sticker_prompt.go` (always includes the emoji hint; when no visual
description exists it says so and tells the model to judge from the emoji).
Sticker packs are managed through the Launcher web UI
(`web/backend/api/sticker.go`, `pkg/gateway/sticker_api.go`, `pkg/sticker/`).

### Sticker pack import (Launcher UI)

Importing a Telegram sticker set (`/api/telegram/stickers/import-set`) downloads each
sticker, generates a description with the default vision model, and registers it.
Notable fixes:
- **Stable IDs.** The sticker ID uses the sticker's index in the set
  (`<setName>_<index>`), so IDs stay unique and stable across re-imports.
- **Correct MIME for descriptions.** The image handed to the vision model uses the
  MIME detected from the file's magic bytes (`sticker.FileToDataURL`), not a
  hard-coded `image/webp` — animated/video stickers are `.jpg` thumbnails, and a
  mismatched MIME made the model reject the request (so descriptions came back empty).
- **Preview endpoint.** `GET /api/media/file?path=...` serves sticker images to the
  UI (path-traversal guarded — only files under the sticker media directory).
- **Partial-failure reporting.** The response carries `imported`/`total` and, on
  partial failure, `failed` + an `error` message that the UI surfaces.

> The AI can send individual stickers from the managed packs; it cannot create or
> import arbitrary sticker packs on its own.

---

## 10. Security

### Dependency vulnerability fixes

The synced upstream pinned Go `1.25.13` and `golang.org/x/net v0.55.0`, which are
affected by 11 known advisories (Go standard library + `x/net`, fixed in Go `1.26.9`
and `x/net v0.60.0`). This fork upgrades:

- `go 1.26.9` (in `go.mod`; CI installs it via `go-version-file: go.mod`)
- `golang.org/x/net v0.60.0`, plus matching `x/crypto`, `x/sys`, `x/term`, `x/text`, `x/sync`

`govulncheck ./...` now reports **no vulnerabilities**.

### Credential hygiene

The upstream repository's history contains hard-coded third-party OAuth client
credentials. This fork keeps **placeholders** (`YOUR_GOOGLE_CLIENT_ID_BASE64`) in
`pkg/auth/oauth.go` and the Antigravity docs, and the upstream code is synced as a
**single squashed commit** so those credential-bearing commits never enter this
repository's history.

---

## 11. Group Chat Attribution & Media Fallback

### Speaker attribution in group chats

Session history stores only `role` / `content` / `media` — no sender field. In group
chats this means that after compaction, messages from several people collapse into
anonymous "user said …" lines, and the model can misattribute an earlier message to
the current speaker.

Fix (`pkg/channels/telegram/telegram.go`): in non-private chats each inbound message
is prefixed with `[Name (@username)]: ` **after** the mention/trigger checks (so it
cannot affect `/command` dispatch or mention matching). The prefix is written into
history, so attribution survives compaction. The dynamic system context explains
that this prefix is a system annotation the model must not imitate in its replies.

### Non-omni models receiving audio/video

Telegram voice notes and videos are inlined as `data:audio/...` / `data:video/...`
for the current turn. A model without audio/video support rejects them.

Fallback (`pkg/agent/pipeline_llm.go`): when the model reports unsupported media
**and the turn's media is audio/video only**, the turn is retried once with the
media stripped, plus a system note telling the model the attachments were dropped,
so the user still gets a text answer.

Images are deliberately **not** downgraded: an unsupported image fails with a clear
message telling the user to configure a vision model, matching upstream behaviour
(tests `TestAgentLoop_VisionUnsupportedErrorReturnsClearFailure` and
`TestAgentLoop_LoadImageFollowUpWithoutImageModelFailsClearly` lock this in).

---

## 12. API Key & Provider Rotation

The upstream multi-key support is **failover only**: the first key is always used
and the next one is tried only after an error. This fork adds optional
**round-robin rotation**.

### How it works

Rotation does not replace the fallback logic — it only changes **which candidate
the chain starts with** on each request:

| Request | Start at | Order tried (on failure) |
|---------|----------|--------------------------|
| 1 | A | A → B → C |
| 2 | B | B → C → A |
| 3 | C | C → A → B |

So failover still works exactly as before; only the entry point rotates.

**Implementation** (`pkg/providers/fallback.go`): `FallbackChain` holds an
`atomic.Uint64` counter. When any candidate opts in (`Rotate`), `ExecuteCandidate`
rotates the candidate slice by `counter % len(candidates)`. If nobody opts in,
the slice is untouched — behaviour is bit-for-bit identical to upstream.

### Configuration

Two optional per-model flags (both default `false`):

```json
{
  "model_name": "my-model",
  "model": "openai/gpt-4o",
  "api_base": "https://api.example.com/v1",
  "api_keys": ["sk-key-1", "sk-key-2", "sk-key-3"],
  "key_rotation": true,
  "provider_rotation": true
}
```

- `key_rotation` — rotate through `api_keys` on every request.
- `provider_rotation` — rotate the starting entry of the fallback chain (useful
  when several entries point at different providers/endpoints).

### Enabling it from the Web UI

Models → edit (or add) a model:

1. **API Key** — the primary key (as before).
2. **Extra API keys (rotation)** — additional keys, one per line or comma-separated.
3. Toggle **API key rotation** and/or **Provider rotation**.

Without extra keys the request payload is identical to previous versions, so
existing configurations keep working unchanged.

### Notes

- Multi-key entries are expanded at load time into `name`, `name__key_1`, … as a
  fallback chain; rotation reuses that expansion, so no new storage format.
- Keys are never returned by the API — the model list only exposes
  `api_key_count`.
- Cooldown / rate-limit tracking still applies per key, so a rate-limited key is
  skipped and the next one is used.

---

## 13. Web Chat File Upload

The web chat previously accepted **images only**. This fork adds generic file
upload.

**Flow:**

1. The composer sends non-image files as base64 `data:` URLs in a `files` array
   (images keep using `media` so they are inlined as multimodal input).
2. `pkg/channels/pico` validates them, writes them under
   `<picoclaw-home>/workspace/uploads/<message-id>/`, and appends
   `[file:/abs/path]` tags to the message.
3. The model reads them with the existing `read_file` tool.

**Why the workspace:** the default config sets `restrict_to_workspace: true`, so
files outside the workspace would be unreadable by `read_file`. Writing into
`workspace/uploads/` needs no security relaxation.

**Limits & safety:**

- Max 10 MB per file (frontend) and `max_media_size` on the backend.
- Filenames are sanitised: separators and `..` are replaced, so a crafted name
  cannot escape the uploads directory.
- Files are persisted **only after** the sender passes the allowlist check, so an
  unauthorised client cannot write to disk.
- Image attachments are rejected on the `files` path (they must use `media`), so
  there is exactly one code path per media kind.

**Backward compatibility:** payloads without a `files` key behave exactly as
before; a message with neither text, inline media, nor files is still rejected as
`empty_content`.

## Known Limitations

### Video Format is MiMo-Only

The `video_url` format used for video input is MiMo-specific. There is no standard OpenAI video type. When using non-MiMo models, video content is silently skipped.

**Workaround:** Configure `agents.defaults.video_model` to a MiMo model for video analysis — the delegation pattern will describe the video and pass the text to the main model.

### Chat API Does Not Accept Multimodal Input

The `/api/chat` endpoint accepts only plain text messages (`{"message": "text"}`). It does not support the OpenAI Messages API format with multipart content (images, audio, video inline). Multimodal content is only supported when sent through channel integrations (Telegram, Discord, etc.) or internal tool results.

---

## Upstream Documentation

For the original PicoClaw project documentation (features, installation, providers, channels, etc.), see:

- **English:** [UPSTREAM-README.md](UPSTREAM-README.md)
- **中文:** [docs/project/UPSTREAM-README.zh.md](docs/project/UPSTREAM-README.zh.md)
