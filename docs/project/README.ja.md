[English](../../README.md) | [中文](README.zh.md) | [한국어](README.ko.md) | [Português](README.pt-br.md) | [Tiếng Việt](README.vi.md) | [Français](README.fr.md) | [Italiano](README.it.md) | [Bahasa Indonesia](README.id.md) | [Malay](README.ms.md) | **日本語**

# PicoClaw 32 ビット Fork — 変更説明とドキュメント

このドキュメントは、此の fork（`picoclaw-edition`）における、上流 [PicoClaw](https://github.com/sipeed/picoclaw) プロジェクトからのすべての変更を説明します。

---

## 目次

- [1. デュアルマルチモーダル API フォーマット対応](#1-デュアルマルチモーダル-api-フォーマット対応)
- [2. ビデオ理解](#2-ビデオ理解)
- [3. Telegram ビデオメッセージ](#3-telegram-ビデオメッセージ)
- [4. load_video ツール](#4-load_video-ツール)
- [5. 音頻 Data URL エンコーディング](#5-音頻-data-url-エンコーディング)
- [6. 設定の堅牢性](#6-設定の堅牢性)
- [7. API 変更](#7-api-変更)
- [8. 32 ビットプラットフォーム対応](#8-32-ビットプラットフォーム対応)
- [既知の制限事項](#既知の制限事項)

---

## 1. デュアルマルチモーダル API フォーマット対応

**問題:** 上流のプロバイダーコードは標準 OpenAI フォーマットで音声を送信しますが、MiMo の API は完全な data URL を要求します。

**解決策:** プロバイダー感知のフォーマット選択を追加し、ターゲットプロバイダーを自動検出して適切なフォーマットを送信します。

**フォーマット比較:**

| タイプ | MiMo フォーマット | 標準 OpenAI フォーマット |
|--------|------------------|------------------------|
| 画像 | `image_url` + data URL | 同左（ユニバーサル） |
| 音声 | `input_audio.data` = 完全な data URL | `input_audio.data` = base64 + `format` フィールド |
| ビデオ | `video_url` + `fps` + `media_resolution` | 標準タイプなし |

---

## 2. ビデオ理解

**問題:** `video_model` 設定フィールドは存在しましたが、エージェントコードで使用されませんでした。

**変更内容:**
- `pkg/agent/llm_media.go` — `describeVideoProxy()` 関数を追加し、**委任パターン**を実装:
  1. 現在のターンで `data:video/` URL を検出
  2. ビデオ + 説明プロンプトを `video_model` に送信
  3. 説明をメッセージコンテンツに注入
  4. メインモデルが説明を使用して回复

---

## 3. Telegram ビデオメッセージ

**問題:** `collectTelegramMessageParts` は Photo、Voice、Audio、Document を処理していましたが、Video は処理していませんでした。

**変更内容:** `pkg/channels/telegram/telegram.go` — `msg.Video` の処理を追加。

---

## 4. load_video ツール

**新機能:** AI がローカルビデオファイルを読み込んで分析できるツール。

**関連ファイル:**
- `pkg/tools/fs/load_video.go` — 新ツール実装
- `pkg/tools/fs_facade.go` — `LoadVideoTool` 型エイリアス
- `pkg/agent/agent_init.go` — ツール登録

---

## 5. 音頻 Data URL エンコーディング

**問題:** ユーザーメッセージの音声が data URL としてエンコードされていませんでした。

**変更内容:** `pkg/agent/agent_media.go` — ユーザーメッセージとツール結果の音声/ビデオを data URL としてエンコード。

---

## 6. 設定の堅牢性

- 未知の設定フィールドがエラーではなく警告に降格
- 設定 API リクエストボディ制限を 1MB → 20MB に増加
- VoiceConfig に MimoConfig フィールドを追加

---

## 7. API 変更

| エンドポイント | 方法 | 説明 |
|---------------|------|------|
| `/api/chat` | POST | 同期チャット |
| `/api/chat/stream` | POST | ストリーミングチャット (SSE) |

詳細: [API リファレンス](../api/README.ja.md)

---

## 8. 32 ビットプラットフォーム対応

| OS | GOARCH | バイナリ名 |
|----|--------|-----------|
| Linux | `386` | `picoclaw-linux-386` |
| Linux | `arm` | `picoclaw-linux-arm` |
| Linux | `mipsle` | `picoclaw-linux-mipsle` |
| Windows | `386` | `picoclaw-windows-386.exe` |

---

## 既知の制限事項

- **ビデオフォーマット:** `video_url` は MiMo 専用。非 MiMo プロバイダーではビデオがスキップされる。
- **チャット API:** テキストのみ。マルチモーダル入力非対応。

---

## 上流ドキュメント

元の PicoClaw プロジェクトドキュメント:
- **English:** [../../UPSTREAM-README.md](../../UPSTREAM-README.md)
- **日本語:** [UPSTREAM-README.ja.md](UPSTREAM-README.ja.md)
