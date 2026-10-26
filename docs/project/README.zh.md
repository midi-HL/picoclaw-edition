[English](../../README.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Português](README.pt-br.md) | [Tiếng Việt](README.vi.md) | [Français](README.fr.md) | [Italiano](README.it.md) | [Bahasa Indonesia](README.id.md) | [Malay](README.ms.md) | **中文**

# PicoClaw 32 位 Fork — 修改说明与文档

这是此 fork（`picoclaw-edition`）的主要文档。包含相对于上游 [PicoClaw](https://github.com/sipeed/picoclaw) 的所有修改说明，以及 32 位平台支持、多模态 API 变更和新工具。

---

## 目录

- [1. MiMo 多模态支持](#1-mimo-多模态支持)
- [2. 视频理解](#2-视频理解)
- [3. Telegram 视频消息](#3-telegram-视频消息)
- [4. load\_video 工具](#4-load_video-工具)
- [5. 音频 Data URL 编码](#5-音频-data-url-编码)
- [6. 配置健壮性](#6-配置健壮性)
- [7. API 变更](#7-api-变更)
- [8. 32 位平台支持](#8-32-位平台支持)
- [9. Telegram 贴纸（自定义表情包）](#9-telegram-贴纸自定义表情包)
- [10. 安全性](#10-安全性)
- [11. 群聊发送者归属与媒体回退](#11-群聊发送者归属与媒体回退)
- [12. API 密钥/服务商轮询](#12-api-密钥服务商轮询)
- [13. Web 对话上传文件](#13-web-对话上传文件)
- [已知限制](#已知限制)

---

## 1. 双重多模态 API 格式支持（标准 OpenAI + MiMo）

**问题：** 上游 provider 代码使用标准 OpenAI 格式发送音频（`{"data": "base64", "format": "wav"}`），但 MiMo API 要求完整 data URL（`{"data": "data:audio/wav;base64,..."}`）。单一格式无法同时满足两种 provider。

**解决方案：** 新增 provider 感知的格式选择，自动检测目标 provider 并发送对应格式。

### 实现方案

**`pkg/providers/common/common.go` 中新增类型和函数：**

```go
// SerializeOptions 携带 provider 身份信息，用于格式特定的序列化。
type SerializeOptions struct {
    ProviderName string  // 如 "mimo"、"openai"、"anthropic"
    APIBase      string  // 如 "https://api.xiaomimimo.com/v1"
}

// Provider 感知的序列化 — 根据目标 provider 选择格式。
func SerializeMessagesWithOptions(messages []Message, opts *SerializeOptions) []any

// 向后兼容 — 默认使用标准 OpenAI 格式。
func SerializeMessages(messages []Message) []any
```

**Provider 检测逻辑：**

```go
func isMiMoProvider(opts *SerializeOptions) bool {
    return opts.ProviderName == "mimo" ||
           strings.Contains(opts.APIBase, "xiaomimimo.com")
}
```

**调用链：**

```
openai_compat.Provider.buildRequestBody()
  → 传递 p.providerName + p.apiBase 作为 SerializeOptions
  → SerializeMessagesWithOptions 根据 provider 选择格式
```

### 按 Provider 的格式选择

| 媒体类型   | MiMo Provider                                                                                                       | 标准 OpenAI Provider                                                               |
| ------ | ------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| **图片** | `{"type": "image_url", "image_url": {"url": "data:image/..."}}`                                                     | 相同（通用格式）                                                                         |
| **音频** | `{"type": "input_audio", "input_audio": {"data": "data:audio/wav;base64,..."}}`                                     | `{"type": "input_audio", "input_audio": {"data": "base64...", "format": "wav"}}` |
| **视频** | `{"type": "video_url", "video_url": {"url": "data:video/mp4;base64,..."}, "fps": 2, "media_resolution": "default"}` | 跳过（无标准类型）                                                                        |

### 关键差异说明

**音频格式拆分：**

- MiMo 要求 **完整 data URL**（`data:audio/wav;base64,...`）放在 `data` 字段，无 `format` 字段
- 标准 OpenAI 要求 **纯 base64** 放在 `data`，MIME 子类型放在单独的 `format` 字段
- 代码使用 `ParseDataAudioURL()` 解析 data URL 以提取两部分

**视频格式：**

- `video_url` 是 MiMo 专用类型，包含 `fps` 和 `media_resolution` 参数
- 标准 OpenAI 无视频类型 — 非 MiMo provider 会静默跳过视频
- 回退方案：`video_model` 代理转述可为不支持视频的模型描述视频内容

### 向后兼容

- `SerializeMessages()`（无 options）→ 标准 OpenAI 格式，与上游一致
- `SerializeMessagesWithOptions(msgs, nil)` → 标准 OpenAI 格式
- `SerializeMessagesWithOptions(msgs, &SerializeOptions{ProviderName: "mimo"})` → MiMo 格式

---

## 2. 视频理解

**问题：** `video_model` 配置字段存在但从未被 agent 代码使用。

**修改内容：**

- `pkg/agent/instance.go` — `AgentInstance` 新增 `VideoCandidates` 字段，启动时解析 `video_model` 候选。
- `pkg/agent/llm_media.go` — 新增 `describeVideoProxy()` 函数，实现**代理转述模式**：
  1. 检测当前 turn 中的 `data:video/` URL
  2. 将视频 + 描述提示词发送给 `video_model`
  3. 将描述注入消息内容：`[系统消息：以下是用户发送视频的描述]`
  4. 主模型基于描述回复用户
- `pkg/agent/llm_media.go` — `routeMediaTurn` 在回退到 image model 路由之前先调用 `describeVideoProxy`。

**流程：**

```
用户发送视频
  → video_model 描述视频
  → 描述注入消息
  → 主模型基于描述回复
```

---

## 3. Telegram 视频消息

**问题：** `collectTelegramMessageParts` 处理了 Photo、Voice、Audio 和 Document，但没有处理 Video。视频消息被静默丢弃。

**修改内容：**

- `pkg/channels/telegram/telegram.go` — 新增 `msg.Video` 处理：下载视频文件、存入 media store、在消息内容中添加 `[video]` 标签。

---

## 4. load_video 工具

**新功能：** 允许 AI 加载和分析本地视频文件的工具。

**相关文件：**

- `pkg/tools/fs/load_video.go` — 新工具实现（与 `load_image` 同构）。
- `pkg/tools/fs_facade.go` — 新增 `LoadVideoTool` 类型别名和 `NewLoadVideoTool` 工厂函数。
- `pkg/agent/agent_init.go` — 注册 `load_video` 工具。
- `pkg/config/config.go` — 新增 `LoadVideo ToolConfig` 字段。
- `pkg/agent/context.go` — 更新多模态系统提示词，提及 `load_video`。

**工作原理：**

1. AI 调用 `load_video(path="video.mp4")`
2. 工具验证路径、检测 MIME 类型、存入 media store
3. 返回 `media://` 引用
4. `resolveMediaRefs` 编码为 `data:video/mp4;base64,...`
5. Provider 以 `video_url` 格式发送给 MiMo

---

## 5. 音频 Data URL 编码

**问题：** 用户消息中的音频未被编码为 data URL。

**修改内容：**

- `pkg/agent/agent_media.go` — `resolveMediaRefs` 现在为用户消息和工具结果中的音频/视频编码为 data URL。
- `pkg/agent/prompt_turn.go` — `toolImageFollowUpPromptMessage` 检测视频 data URL 并相应更新合成用户消息文本。

---

## 6. 配置健壮性

### 未知字段降级为警告

**问题：** 包含已弃用字段的配置文件（如旧版本）导致启动失败。

**修改内容：**

- `pkg/config/diagnostics.go` — `decodeJSONWithDiagnostics` 现在将未知字段记录为 stderr 警告，而不是返回错误。

### 配置 API 请求体限制

**问题：** PATCH/PUT `/api/config` 端点的请求体限制为 1MB，对于 base64 编码的音色复刻音频数据来说太小。

**修改内容：**

- `web/backend/api/config.go` — PUT 和 PATCH 处理器的请求体限制从 1MB 提高到 20MB。

### VoiceConfig MimoConfig 字段

**问题：** Go 的 `VoiceConfig` 结构体没有 `MimoConfig` 字段，导致 MiMo 特定设置在 JSON 序列化/反序列化时丢失。

**修改内容：**

- `pkg/config/config.go` — 新增 `VoiceMimoConfig` 结构体，包含 ASR 字段（`asr_provider`、`asr_language`、`asr_api_key`）。

---

## 7. API 变更

此 fork 新增了以下 API 能力。详细 API 文档请参见 [API 参考文档](../api/README.zh.md)。

### 聊天 API

| 端点                 | 方法   | 说明                     |
| ------------------ | ---- | ---------------------- |
| `/api/chat`        | POST | 同步聊天 — 发送消息，接收完整回复     |
| `/api/chat/stream` | POST | 流式聊天 — SSE 实时 token 输出 |

### 新工具：load_video

`load_video` 工具注册为 AI agent 的可调用工具。接受文件路径参数，返回视频分析引用。

**工具参数：**

```json
{
  "path": "path/to/video.mp4"
}
```

**工具结果：**

```
Video loaded: video.mp4
[video: /path/to/video.mp4]
```

视频会自动编码为 data URL，并以 `video_url` 格式发送给模型。

---

## 8. 32 位平台支持

> 本文档记录此 fork 新增的 32 位平台支持。

### 支持的 32 位目标平台

| 操作系统    | GOARCH          | 二进制文件名                     | 最低系统版本                                           |
| ------- | --------------- | -------------------------- | ------------------------------------------------ |
| Linux   | `386`           | `picoclaw-linux-386`       | 任何内核 2.6+ 的 32 位 Linux                           |
| Linux   | `arm` (GOARM=7) | `picoclaw-linux-arm`       | ARMv7 Linux（如树莓派）                                |
| Linux   | `mipsle`        | `picoclaw-linux-mipsle`    | MIPS32 小端序 Linux（软浮点）                            |
| Linux   | `mips`          | `picoclaw-linux-mips`      | MIPS32 大端序 Linux（软浮点）                            |
| Windows | `386`           | `picoclaw-windows-386.exe` | Windows XP SP3 / Vista / 7 / 8 / 8.1 / 10 (32 位) |

### 修改内容

- 在 Makefile 的 `build-all` 和 `build-whatsapp-native` 目标中新增了 `linux/386`、`linux/arm`、`linux/mipsle`、`linux/mips` 构建目标
- `windows/386` 目标已存在于 Makefile 和 `.goreleaser.yaml` 中
- 使用 modernc sqlite/libc 的源文件已添加构建标签排除 `mipsle` 和 `mips` 大端序

### 实现方式

- 通过 `goolm` 构建标签使用纯 Go 实现的 olm 加密库，无需 CGO / `libolm` 依赖
- 所有使用的 Windows API 均为 Vista/Win7 级别，无 Win10+ 专有 API
- `unsafe.Pointer` 的使用与架构无关
- 飞书/Lark 频道在 32 位平台上**不可用**（上游 SDK 限制，运行时会优雅处理）
- Matrix 频道在 MIPS（LE 和 BE）上**不可用**，受 modernc sqlite/libc 构建限制
- MIPS 目标使用 `GOMIPS=softfloat` 以兼容无浮点单元的内核

### 从源码编译

```bash
# Linux 32 位 x86
CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -v -tags goolm,stdjson -o build/picoclaw-linux-386 ./cmd/picoclaw

# Linux 32 位 ARM (GOARM=7)
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -v -tags goolm,stdjson -o build/picoclaw-linux-arm ./cmd/picoclaw

# Linux 32 位 MIPS 小端序（软浮点，无 goolm）
CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -v -tags stdjson -o build/picoclaw-linux-mipsle ./cmd/picoclaw

# Linux 32 位 MIPS 大端序（软浮点，无 goolm）
CGO_ENABLED=0 GOOS=linux GOARCH=mips GOMIPS=softfloat go build -v -tags stdjson -o build/picoclaw-linux-mips ./cmd/picoclaw

# Windows 32 位（可从任意操作系统交叉编译）
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build -v -tags goolm,stdjson -o build/picoclaw-windows-386.exe ./cmd/picoclaw

# 或使用 Makefile 目标（构建所有平台，包括 32 位）：
make build-all
```

### 运行时系统要求

| 资源  | 最低要求                  |
| --- | --------------------- |
| CPU | 任何支持 SSE2 的 x86 处理器   |
| 内存  | 512 MB                |
| 磁盘  | 100 MB（二进制文件）+ 工作空间存储 |
| 网络  | 需要互联网访问以调用 LLM API    |

---

## 9. Telegram 贴纸（自定义表情包）

本 Fork 新增 **双向** Telegram 贴纸支持。

### 接收方向 —— 用户给 AI 发贴纸

用户发送 Telegram 贴纸时，机器人会把它作为真实图片转发给模型，让 AI 能够**看到并理解**贴纸。

**问题：** 用户发送的图片通常会被 `resolveMediaRefs` 转成路径标签（`[image:/path]`），图片数据**不会**发给模型——模型需自行调用 `load_image` 工具查看。因此贴纸对模型不可见（只看到路径），无法对贴纸做出反应。

**修复：**

- `pkg/channels/telegram/telegram.go` —— 下载贴纸（动画/视频贴纸降级为其静态 `.jpg` 缩略图；普通贴纸用 `.webp`），并**以专用来源标记**（`media.SourceTelegramSticker`）注册进媒体存储。消息文本附带明确的系统提示，告知模型这是一张 **Telegram 贴纸 / 自定义表情包**，*不是*用户上传的照片。
- `pkg/agent/agent_media.go` —— `resolveMediaRefs` 在**当前轮**把**贴纸**媒体（以及用户语音/视频）内联为 `data:...;base64,` URL，MIME 取自文件 magic bytes。普通用户图片保持上游行为（仅路径标签 + 按需 `load_image`），因此大图不会被重复内联发送。
- 健壮性 —— 媒体引用会被**持久化进会话历史**，所以内联 base64 绝不能存进历史（否则每轮都会重发、越来越臃肿）。贴纸以小的引用存储、仅在发送时内联；此外 `resolveMediaRefs` 会丢弃**历史轮次**中的内联 `data:` 引用，可自动修复被旧版本污染的会话。

### 发送方向 —— AI 发贴纸

AI 通过专用的 **`send_sticker` 工具**（`pkg/tools/integration/sticker.go`，在 `pkg/agent/agent_init.go` 注册）发送贴纸，而**不再**在回复正文里写文本标记。

**为什么改用工具（而不是旧的 `[SEND_STICKER: id]` 文本标记）：** 旧方案在频道层解析模型回复文本，十分脆弱：

- 出站文本会在**到达频道之前**就被切片，靠近 4000 字边界的标记可能被切成两半而静默丢失；
- 模型永远不知道贴纸是否真的发出去了，于是会谎称"已发送"；
- 失败只出现在服务器日志里。

工具会把真实的成功/失败结果返回给模型，因此它始终知道结果。工具描述中已标明**仅限 Telegram**。

配套行为：

- **旧标记只剥离、不执行。** 若旧会话让模型仍输出 `[SEND_STICKER: id]`，`stripLegacyStickerTag` 会把它从可见文本中移除（`TelegramChannel.Send` 与流式终结器两处都处理），用户不会看到裸标记；贴纸只通过工具发送。
- **模糊 ID 兜底。** 若 ID 的分隔符丢失，匹配时会忽略 `_` 与 `-`。
- **file_id 失败回退本地文件。** Telegram `file_id` 是 bot 级作用域的；用缓存的 `file_id` 发送被拒时，频道改为上传本地文件（仅当错误表明 ID 确实无效时才清除缓存）。
- **跨机器路径修复。** 贴纸库存的是绝对路径；若库从其他机器迁移过来，会按文件名在当前媒体目录内重新定位（发送与预览均适用）。

可用贴纸列表由 `pkg/agent/sticker_prompt.go` 注入系统提示词（始终包含 emoji；若没有画面描述会明确说明，让模型仅凭 emoji 判断）。贴纸包通过 Launcher Web 界面管理（`web/backend/api/sticker.go`、`pkg/gateway/sticker_api.go`、`pkg/sticker/`）。

### 贴纸包导入（Launcher 界面）

导入 Telegram 贴纸集（`/api/telegram/stickers/import-set`）会下载每张贴纸、用默认视觉模型生成描述并注册。相关修复：

- **稳定 ID。** 贴纸 ID 用其在集合中的下标（`<setName>_<index>`），重复导入时 ID 保持唯一稳定。
- **描述 MIME 正确。** 交给视觉模型的图片使用**从文件 magic bytes 检测**的 MIME（`sticker.FileToDataURL`），而非硬编码 `image/webp`——动画/视频贴纸是 `.jpg` 缩略图，MIME 不符会让模型拒绝请求（导致描述为空）。
- **预览端点。** `GET /api/media/file?path=...` 为界面提供贴纸图片（已做路径穿越防护——仅限贴纸媒体目录下的文件）。
- **部分失败上报。** 响应含 `imported`/`total`，部分失败时含 `failed` + `error`，界面会展示。

> AI 可以从已管理的贴纸包中发送单张贴纸；但它无法自行创建或导入任意贴纸包。

---

## 10. 安全性

### 依赖漏洞修复

同步的上游锁定了 Go `1.25.13` 与 `golang.org/x/net v0.55.0`，受 11 个已知安全公告影响（Go 标准库 + `x/net`，已在 Go `1.26.9` 与 `x/net v0.60.0` 修复）。本 Fork 升级：

- `go 1.26.9`（写入 `go.mod`；CI 通过 `go-version-file: go.mod` 安装）
- `golang.org/x/net v0.60.0`，并同步升级 `x/crypto`、`x/sys`、`x/term`、`x/text`、`x/sync`

`govulncheck ./...` 现在报告 **无漏洞**。

### 凭据卫生

上游仓库历史中含硬编码的第三方 OAuth 客户端凭据。本 Fork 在 `pkg/auth/oauth.go` 及 Antigravity 文档中保留**占位符**（`YOUR_GOOGLE_CLIENT_ID_BASE64`），并以**单个压缩提交**同步上游代码，使那些携带凭据的提交不会进入本仓库历史。

---

## 11. 群聊发送者归属与媒体回退

### 群聊中的说话人归属

会话历史只存 `role` / `content` / `media`，**没有 sender 字段**。群聊压缩之后，多人的话会被拍平成匿名的"用户说…"，模型可能把早先的消息错认成当前说话人。

修复（`pkg/channels/telegram/telegram.go`）：非私聊中，每条入站消息在**触发/提及判断之后**加上前缀 `[名字 (@username)]: `（因此不影响 `/命令` 分发与 @ 匹配）。前缀会写入历史，归属在压缩后依然保留。动态系统上下文会说明该前缀是系统标注，模型回复时**不得模仿**。

### 非全模态模型收到音频/视频

Telegram 语音与视频会在当前轮内联为 `data:audio/...` / `data:video/...`。不支持音频/视频的模型会拒绝。

回退（`pkg/agent/pipeline_llm.go`）：当模型报告不支持媒体、**且该轮媒体仅含音频/视频**时，剥离媒体重试一次，并附系统提示说明附件已忽略，用户仍能得到文字回答。

图片**有意不降级**：不支持图片时直接报错并提示配置视觉模型，与上游行为一致（由 `TestAgentLoop_VisionUnsupportedErrorReturnsClearFailure` 与 `TestAgentLoop_LoadImageFollowUpWithoutImageModelFailsClearly` 锁定）。

---

## 12. API 密钥/服务商轮询

### 轮询（可选开关）

两个可选的模型级开关：

| 开关 | 作用 |
|---|---|
| `key_rotation: true` | 每次请求轮换 `api_keys` 中的密钥（round-robin），而非固定用第一个、出错才切换。 |
| `provider_rotation: true` | 每次请求轮换 fallback 链的起始位置，让多个服务商/端点分摊负载。 |

```json
{
  "model_name": "gpt-main",
  "model": "openai/gpt-5.5",
  "api_keys": ["sk-a", "sk-b", "sk-c"],
  "key_rotation": true,
  "fallbacks": ["backup-endpoint"],
  "provider_rotation": true
}
```

**默认向下兼容。** 两个开关默认 `false`，且只有在至少一个候选开启时才生效（`anyRotating`）。旧配置行为完全不变：始终从第一个密钥/服务商开始，出错才故障转移。轮询只改变**起始点**，故障转移顺序仍是稳定的环，失败候选照样跳到下一个。

### 实现要点

轮询不替换故障转移，只改变每次请求的**起点**：

| 请求 | 起点 | 失败后依次尝试 |
|---|---|---|
| 1 | A | A → B → C |
| 2 | B | B → C → A |
| 3 | C | C → A → B |

`FallbackChain` 持有一个 `atomic.Uint64` 计数器；只要任一候选开启 `Rotate`，`ExecuteCandidate` 就按 `计数 % 候选数` 旋转切片；无人开启时切片原样不动（与上游逐字节一致）。

### 在 Web 界面开启

模型 → 编辑（或新增）模型：

1. **API Key** —— 主密钥（与以前相同）。
2. **额外 API Key（轮询）** —— 追加的密钥，每行一个或用逗号分隔。
3. 打开 **API Key 轮询** 和/或 **服务商轮询** 开关。

不填额外密钥时，请求体与旧版本完全一致，现有配置无需改动。

---

## 13. Web 对话上传文件

Web 对话输入框现在支持任意文件（不再仅限图片）：

- **图片**沿用原路径——作为多模态媒体内联（`media` 字段），行为不变。
- **其他文件**通过新增的可选 `files` 字段以 `{filename, content_type, data}` data URL 发送，写入 `<workspace>/uploads/<messageID>/`，并以 `[file:/path]` 标签提供给模型，用 `read_file` 读取。

设计要点：

- 文件**存放在 agent 工作区内**：默认 `restrict_to_workspace` 为 `true`，若放系统临时目录，`read_file` 读不到（除非放宽沙箱）。
- 文件写入在**发送者通过 allow-list 校验之后**才进行。
- 文件名做净化（不含分隔符、不含 `..`）；单文件 10 MB 上限。
- 允许「只有文件、没有文字」的消息，不会被判为空内容拒绝。
- 不带 `files` 字段的旧 payload 行为完全不变。

---

## 已知限制

### 视频格式仅限 MiMo

`video_url` 格式是 MiMo 专用的，OpenAI API 中无此类型。使用非 MiMo 模型时，视频内容会被静默跳过。

**解决方案：** 配置 `agents.defaults.video_model` 为 MiMo 模型，使用代理转述模式分析视频。

### 聊天 API 不支持多模态输入

`/api/chat` 端点仅接受纯文本消息（`{"message": "文本"}`），不支持 OpenAI Messages API 的多部分内容格式（内联图片、音频、视频）。多模态内容仅支持通过频道集成（Telegram、Discord 等）或内部工具结果发送。

---

## 上游文档

原始 PicoClaw 项目文档（功能、安装、Provider、频道等），请参见：

- **English:** [../../UPSTREAM-README.md](../../UPSTREAM-README.md)
- **中文:** [UPSTREAM-README.zh.md](UPSTREAM-README.zh.md)
