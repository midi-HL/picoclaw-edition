[English](../../README.md) | [中文](README.zh.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Português](README.pt-br.md) | [Tiếng Việt](README.vi.md) | [Français](README.fr.md) | [Bahasa Indonesia](README.id.md) | [Malay](README.ms.md) | **Italiano**

# PicoClaw Fork 32-bit — Note di Modifica e Documentazione

Questo documento descrive tutte le modifiche apportate a questo fork (`picoclaw-edition`) rispetto al progetto upstream [PicoClaw](https://github.com/sipeed/picoclaw).

---

## Indice

- [1. Supporto Formati Dupli API Multimodale](#1-supporto-formati-dupli-api-multimodale)
- [2. Comprensione Video](#2-comprensione-video)
- [3. Messaggi Video Telegram](#3-messaggi-video-telegram)
- [4. Strumento load_video](#4-strumento-load_video)
- [5. Codifica Data URL Audio](#5-codifica-data-url-audio)
- [6. Robustezza Configurazione](#6-robustezza-configurazione)
- [7. Modifiche API](#7-modifiche-api)
- [8. Supporto Piattaforme 32 bit](#8-supporto-piattaforme-32-bit)
- [Limitazioni Note](#limitazioni-note)

---

## 1. Supporto Formati Dupli API Multimodale

**Problema:** Il codice upstream invia audio nel formato OpenAI standard, ma l'API MiMo si aspetta una data URL completa.

**Soluzione:** Aggiunta selezione formato consapevole del provider.

| Tipo | Formato MiMo | Formato OpenAI Standard |
|------|-------------|------------------------|
| Immagine | `image_url` + data URL | Identico (universale) |
| Audio | `input_audio.data` = data URL completa | `input_audio.data` = base64 + campo `format` |
| Video | `video_url` + `fps` + `media_resolution` | Nessun tipo standard |

---

## 2. Comprensione Video

**Modifiche:** `pkg/agent/llm_media.go` — Funzione `describeVideoProxy()` aggiunta con pattern delega.

---

## 3. Messaggi Video Telegram

**Modifiche:** `pkg/channels/telegram/telegram.go` — Gestione `msg.Video` aggiunta.

---

## 4. Strumento load_video

**Nuova funzionalità:** Strumento che permette all'AI di caricare e analizzare file video locali.

---

## 5. Codifica Data URL Audio

**Modifiche:** `pkg/agent/agent_media.go` — Audio/video nei messaggi utente e nei risultati degli strumenti ora codificati come data URL.

---

## 6. Robustezza Configurazione

- Campi di configurazione sconosciuti ora sono avvisi, non errori
- Limite dimensione corpo richiesta API configurazione aumentato da 1MB a 20MB

---

## 7. Modifiche API

Documentazione dettagliata: [Riferimento API](../api/README.it.md)

---

## 8. Supporto Piattaforme 32 bit

| OS | GOARCH | Nome Binario |
|----|--------|-------------|
| Linux | `386` | `picoclaw-linux-386` |
| Linux | `arm` | `picoclaw-linux-arm` |
| Windows | `386` | `picoclaw-windows-386.exe` |

---

## Limitazioni Note

- **Formato video:** `video_url` è specifico per MiMo.
- **API chat:** Solo testo. Input multimodale non supportato.

---

## Documentazione Upstream

Documentazione originale del progetto PicoClaw:
- **English:** [../../UPSTREAM-README.md](../../UPSTREAM-README.md)
- **Italiano:** [UPSTREAM-README.it.md](UPSTREAM-README.it.md)
