[English](../../README.md) | [中文](README.zh.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Português](README.pt-br.md) | [Tiếng Việt](README.vi.md) | [Italiano](README.it.md) | [Bahasa Indonesia](README.id.md) | [Malay](README.ms.md) | **Français**

# PicoClaw Fork 32-bit — Notes de Modification et Documentation

Ce document décrit toutes les modifications apportées à ce fork (`picoclaw-edition`) par rapport au projet amont [PicoClaw](https://github.com/sipeed/picoclaw).

---

## Table des matières

- [1. Support des Formats Doubles d'API Multimodale](#1-support-des-formats-doubles-dapi-multimodale)
- [2. Compréhension Vidéo](#2-compréhension-vidéo)
- [3. Messages Vidéo Telegram](#3-messages-vidéo-telegram)
- [4. Outil load_video](#4-outil-load_video)
- [5. Encodage Data URL Audio](#5-encodage-data-url-audio)
- [6. Robustesse de la Configuration](#6-robustesse-de-la-configuration)
- [7. Modifications de l'API](#7-modifications-de-lapi)
- [8. Support des Plateformes 32 bits](#8-support-des-plateformes-32-bits)
- [Limitations Connues](#limitations-connues)

---

## 1. Support des Formats Doubles d'API Multimodale

**Problème:** Le code amont envoie l'audio au format OpenAI standard, mais l'API MiMo attend une data URL complète.

**Solution:** Ajout d'une sélection de format consciente du fournisseur.

| Type | Format MiMo | Format OpenAI Standard |
|------|------------|----------------------|
| Image | `image_url` + data URL | Identique (universel) |
| Audio | `input_audio.data` = data URL complète | `input_audio.data` = base64 + champ `format` |
| Vidéo | `video_url` + `fps` + `media_resolution` | Pas de type standard |

---

## 2. Compréhension Vidéo

**Modifications:** `pkg/agent/llm_media.go` — Fonction `describeVideoProxy()` ajoutée avec le pattern délégation.

---

## 3. Messages Vidéo Telegram

**Modifications:** `pkg/channels/telegram/telegram.go` — Gestion de `msg.Video` ajoutée.

---

## 4. Outil load_video

**Nouvelle fonctionnalité:** Outil permettant à l'AI de charger et analyser des fichiers vidéo locaux.

---

## 5. Encodage Data URL Audio

**Modifications:** `pkg/agent/agent_media.go` — L'audio/vidéo dans les messages utilisateur et les résultats d'outils est maintenant encodé en data URLs.

---

## 6. Robustesse de la Configuration

- Les champs de configuration inconnus sont désormais des avertissements, pas des erreurs
- Limite de taille du corps de requête API de configuration augmentée de 1MB à 20MB

---

## 7. Modifications de l'API

Documentation détaillée: [Référence API](../api/README.fr.md)

---

## 8. Support des Plateformes 32 bits

| OS | GOARCH | Nom du Binaire |
|----|--------|---------------|
| Linux | `386` | `picoclaw-linux-386` |
| Linux | `arm` | `picoclaw-linux-arm` |
| Windows | `386` | `picoclaw-windows-386.exe` |

---

## Limitations Connues

- **Format vidéo:** `video_url` est spécifique à MiMo.
- **API chat:** Texte uniquement. Entrée multimodale non supportée.

---

## Documentation Amont

Documentation originale du projet PicoClaw:
- **English:** [../../UPSTREAM-README.md](../../UPSTREAM-README.md)
- **Français:** [UPSTREAM-README.fr.md](UPSTREAM-README.fr.md)
