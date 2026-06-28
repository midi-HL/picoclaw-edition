[English](../../README.md) | [中文](README.zh.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Português](README.pt-br.md) | [Français](README.fr.md) | [Italiano](README.it.md) | [Bahasa Indonesia](README.id.md) | [Malay](README.ms.md) | **Tiếng Việt**

# PicoClaw Fork 32-bit — Ghi chú Sửa đổi và Tài liệu

Tài liệu này mô tả tất cả các thay đổi được thực hiện trong fork này (`picoclaw-edition`) so với dự án upstream [PicoClaw](https://github.com/sipeed/picoclaw).

---

## Mục lục

- [1. Hỗ trợ Định dạng API Đa phương tiện kép](#1-hỗ-trợ-định-dạng-api-đa-phương-tiện-kép)
- [2. Hiểu Video](#2-hiểu-video)
- [3. Tin nhắn Video Telegram](#3-tin-nhắn-video-telegram)
- [4. Công cụ load_video](#4-công-cụ-load_video)
- [5. Mã hóa Data URL Âm thanh](#5-mã-hóa-data-url-âm-thanh)
- [6. Tính bền vững Cấu hình](#6-tính-bền-vững-cấu-hình)
- [7. Thay đổi API](#7-thay-đổi-api)
- [8. Hỗ trợ Nền tảng 32-bit](#8-hỗ-trợ-nền-tảng-32-bit)
- [Hạn chế đã biết](#hạn-chế-đã-biết)

---

## 1. Hỗ trợ Định dạng API Đa phương tiện kép

**Vấn đề:** Mã upstream gửi âm thanh theo định dạng OpenAI chuẩn, nhưng API MiMo yêu cầu data URL đầy đủ.

**Giải pháp:** Thêm chọn định dạng nhận biết nhà cung cấp.

**So sánh định dạng:**

| Loại | Định dạng MiMo | Định dạng OpenAI chuẩn |
|------|---------------|----------------------|
| Hình ảnh | `image_url` + data URL | Giống nhau (phổ quát) |
| Âm thanh | `input_audio.data` = data URL đầy đủ | `input_audio.data` = base64 + trường `format` |
| Video | `video_url` + `fps` + `media_resolution` | Không có loại chuẩn |

---

## 2. Hiểu Video

**Thay đổi:** `pkg/agent/llm_media.go` — Thêm hàm `describeVideoProxy()` với mẫu ủy quyền.

---

## 3. Tin nhắn Video Telegram

**Thay đổi:** `pkg/channels/telegram/telegram.go` — Thêm xử lý `msg.Video`.

---

## 4. Công cụ load_video

**Tính năng mới:** Công cụ cho phép AI tải và phân tích tệp video cục bộ.

---

## 5. Mã hóa Data URL Âm thanh

**Thay đổi:** `pkg/agent/agent_media.go` — Âm thanh/video trong tin nhắn người dùng và kết quả công cụ现在 được mã hóa dưới dạng data URL.

---

## 6. Tính bền vững Cấu hình

- Trường cấu hình không xác định giờ là cảnh báo, không phải lỗi
- Giới hạn kích thước body yêu cầu API cấu hình tăng từ 1MB lên 20MB

---

## 7. Thay đổi API

Tài liệu chi tiết: [Tham chiếu API](../api/README.vi.md)

---

## 8. Hỗ trợ Nền tảng 32-bit

| OS | GOARCH | Tên Binary |
|----|--------|-----------|
| Linux | `386` | `picoclaw-linux-386` |
| Linux | `arm` | `picoclaw-linux-arm` |
| Windows | `386` | `picoclaw-windows-386.exe` |

---

## Hạn chế đã biết

- **Định dạng video:** `video_url` chỉ dành cho MiMo.
- **API chat:** Chỉ văn bản. Không hỗ trợ đầu vào đa phương tiện.

---

## Tài liệu Upstream

Tài liệu dự án PicoClaw gốc:
- **English:** [../../UPSTREAM-README.md](../../UPSTREAM-README.md)
- **Tiếng Việt:** [UPSTREAM-README.vi.md](UPSTREAM-README.vi.md)
