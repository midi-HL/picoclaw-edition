[English](../../README.md) | [中文](README.zh.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Português](README.pt-br.md) | [Tiếng Việt](README.vi.md) | [Français](README.fr.md) | [Italiano](README.it.md) | [Bahasa Indonesia](README.id.md) | **Malay**

# PicoClaw Fork 32-bit — Nota Pengubahsuaian dan Dokumentasi

Dokumen ini menerangkan semua pengubahsuaian yang dilakukan pada fork ini (`picoclaw-edition`) berbanding projek upstream [PicoClaw](https://github.com/sipeed/picoclaw).

---

## Jadual Kandungan

- [1. Sokongan Format Ganda API Multimodal](#1-sokongan-format-ganda-api-multimodal)
- [2. Pemahaman Video](#2-pemahaman-video)
- [3. Mesej Video Telegram](#3-mesej-video-telegram)
- [4. Alat load_video](#4-alat-load_video)
- [5. Pengekodan Data URL Audio](#5-pengekodan-data-url-audio)
- [6. Kekokohan Konfigurasi](#6-kekokohan-konfigurasi)
- [7. Perubahan API](#7-perubahan-api)
- [8. Sokongan Platform 32-bit](#8-sokongan-platform-32-bit)
- [Had yang Diketahui](#had-yang-diketahui)

---

## 1. Sokongan Format Ganda API Multimodal

**Masalah:** Kod upstream menghantar audio dalam format OpenAI piawai, tetapi API MiMo memerlukan data URL penuh.

**Penyelesaian:** Penambahan pemilihan format yang menyedari pembekal.

| Jenis | Format MiMo | Format OpenAI Piawai |
|-------|------------|---------------------|
| Imej | `image_url` + data URL | Sama (sejagat) |
| Audio | `input_audio.data` = data URL penuh | `input_audio.data` = base64 + medan `format` |
| Video | `video_url` + `fps` + `media_resolution` | Tiada jenis piawai |

---

## 2. Pemahaman Video

**Perubahan:** `pkg/agent/llm_media.go` — Fungsi `describeVideoProxy()` ditambah dengan corak delegasi.

---

## 3. Mesej Video Telegram

**Perubahan:** `pkg/channels/telegram/telegram.go` — Pengendalian `msg.Video` ditambah.

---

## 4. Alat load_video

**Ciri baharu:** Alat yang membolehkan AI memuatkan dan menganalisis fail video tempatan.

---

## 5. Pengekodan Data URL Audio

**Perubahan:** `pkg/agent/agent_media.go` — Audio/video dalam mesej pengguna dan hasil alat kini dikodkan sebagai data URL.

---

## 6. Kekokohan Konfigurasi

- Medan konfigurasi yang tidak diketahui kini menjadi amaran, bukan ralat
- Had saiz badan permintaan API konfigurasi dinaikkan dari 1MB kepada 20MB

---

## 7. Perubahan API

Dokumentasi terperinci: [Rujukan API](../api/README.ms.md)

---

## 8. Sokongan Platform 32-bit

| OS | GOARCH | Nama Binary |
|----|--------|------------|
| Linux | `386` | `picoclaw-linux-386` |
| Linux | `arm` | `picoclaw-linux-arm` |
| Windows | `386` | `picoclaw-windows-386.exe` |

---

## Had yang Diketahui

- **Format video:** `video_url` khusus untuk MiMo.
- **API chat:** Teks sahaja. Input multimodal tidak disokong.

---

## Dokumentasi Upstream

Dokumentasi asal projek PicoClaw:
- **English:** [../../UPSTREAM-README.md](../../UPSTREAM-README.md)
- **Malay:** [UPSTREAM-README.ms.md](UPSTREAM-README.ms.md)
