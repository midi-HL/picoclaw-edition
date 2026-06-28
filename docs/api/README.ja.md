[English](README.md) | [中文](README.zh.md) | [한국어](README.ko.md) | [Português](README.pt-br.md) | [Tiếng Việt](README.vi.md) | [Français](README.fr.md) | [Italiano](README.it.md) | [Bahasa Indonesia](README.id.md) | [Malay](README.ms.md) | **日本語**

# PicoClaw Fork — API リファレンス

このドキュメントは、此の fork で利用可能な API エンドポイントを説明します。サードパーティクライアント開発に役立つエンドポイントに焦点を当てています。

---

## アーキテクチャ概要

| サーバー | デフォルトポート | 用途 |
|---------|---------------|------|
| **Launcher** | 18800 | ダッシュボード UI、API プロキシ、設定管理 |
| **Gateway** | 18790 | コアチャット API、ヘルスチェック |

---

## 主要 API エンドポイント

| エンドポイント | 方法 | 説明 |
|---------------|------|------|
| `/api/chat` | POST | 同期チャット |
| `/api/chat/stream` | POST | ストリーミングチャット (SSE) |
| `/api/sessions` | GET | セッション一覧 |
| `/api/sessions/{id}` | GET | セッション履歴取得 |
| `/api/models` | GET | モデル一覧 |
| `/api/config` | GET/PATCH | 設定の読み書き |
| `/api/gateway/status` | GET | ゲートウェイ状態 |
| `/health` | GET | ヘルスチェック |

---

詳細は英語版をご参照ください: [English API Reference](README.md)
