[English](README.md) | [中文](README.zh.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Português](README.pt-br.md) | [Français](README.fr.md) | [Italiano](README.it.md) | [Bahasa Indonesia](README.id.md) | [Malay](README.ms.md) | **Tiếng Việt**

# PicoClaw Fork — Tham chiếu API

Tài liệu này mô tả các endpoint API có sẵn trong fork này. Tập trung vào các endpoint hữu ích cho phát triển client bên thứ ba.

---

## Các Endpoint API Chính

| Endpoint | Phương thức | Mô tả |
|----------|------------|-------|
| `/api/chat` | POST | Chat đồng bộ |
| `/api/chat/stream` | POST | Chat streaming (SSE) |
| `/api/sessions` | GET | Danh sách phiên |
| `/api/sessions/{id}` | GET | Lịch sử phiên |
| `/api/models` | GET | Danh sách mô hình |
| `/api/config` | GET/PATCH | Đọc/ghi cấu hình |
| `/api/gateway/status` | GET | Trạng thái gateway |
| `/health` | GET | Health check |

---

Chi tiết xem phiên bản tiếng Anh: [English API Reference](README.md)
