[English](../../README.md) | [中文](README.zh.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Português](README.pt-br.md) | [Tiếng Việt](README.vi.md) | [Français](README.fr.md) | [Italiano](README.it.md) | [Malay](README.ms.md) | **Bahasa Indonesia**

# PicoClaw Fork 32-bit — Catatan Modifikasi dan Dokumentasi

Dokumen ini menjelaskan semua modifikasi yang dilakukan pada fork ini (`picoclaw-edition`) dibandingkan dengan proyek upstream [PicoClaw](https://github.com/sipeed/picoclaw).

---

## Daftar Isi

- [1. Dukungan Format Ganda API Multimodal](#1-dudukungan-format-ganda-api-multimodal)
- [2. Pemahaman Video](#2-pemahaman-video)
- [3. Pesan Video Telegram](#3-pesan-video-telegram)
- [4. Alat load_video](#4-alat-load_video)
- [5. Pengodean Data URL Audio](#5-pengodean-data-url-audio)
- [6. Ketahanan Konfigurasi](#6-ketahanan-konfigurasi)
- [7. Perubahan API](#7-perubahan-api)
- [8. Dukungan Platform 32-bit](#8-dundukan-platform-32-bit)
- [Keterbatasan yang Diketahui](#keterbatasan-yang-diketahui)

---

## 1. Dukungan Format Ganda API Multimodal

**Masalah:** Kode upstream mengirim audio dalam format OpenAI standar, tetapi API MiMo meminta data URL lengkap.

**Solusi:** Penambahan pemilihan format yang menyadari provider.

| Tipe | Format MiMo | Format OpenAI Standar |
|------|------------|----------------------|
| Gambar | `image_url` + data URL | Sama (universal) |
| Audio | `input_audio.data` = data URL lengkap | `input_audio.data` = base64 + kolom `format` |
| Video | `video_url` + `fps` + `media_resolution` | Tidak ada tipe standar |

---

## 2. Pemahaman Video

**Perubahan:** `pkg/agent/llm_media.go` — Fungsi `describeVideoProxy()` ditambahkan dengan pola delegasi.

---

## 3. Pesan Video Telegram

**Perubahan:** `pkg/channels/telegram/telegram.go` — Penanganan `msg.Video` ditambahkan.

---

## 4. Alat load_video

**Fitur baru:** Alat yang memungkinkan AI memuat dan menganalisis file video lokal.

---

## 5. Pengodean Data URL Audio

**Perubahan:** `pkg/agent/agent_media.go` — Audio/video dalam pesan pengguna dan hasil alat sekarang dikodekan sebagai data URL.

---

## 6. Ketahanan Konfigurasi

- Field konfigurasi yang tidak dikenal sekarang menjadi peringatan, bukan error
- Batas ukur body request API konfigurasi ditingkatkan dari 1MB menjadi 20MB

---

## 7. Perubahan API

Dokumentasi detail: [Referensi API](../api/README.id.md)

---

## 8. Dukungan Platform 32-bit

| OS | GOARCH | Nama Binary |
|----|--------|------------|
| Linux | `386` | `picoclaw-linux-386` |
| Linux | `arm` | `picoclaw-linux-arm` |
| Windows | `386` | `picoclaw-windows-386.exe` |

---

## Keterbatasan yang Diketahui

- **Format video:** `video_url` khusus untuk MiMo.
- **API chat:** Hanya teks. Input multimodal tidak didukung.

---

## Dokumentasi Upstream

Dokumentasi asli proyek PicoClaw:
- **English:** [../../UPSTREAM-README.md](../../UPSTREAM-README.md)
- **Bahasa Indonesia:** [UPSTREAM-README.id.md](UPSTREAM-README.id.md)
