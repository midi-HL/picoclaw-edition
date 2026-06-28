[English](../../README.md) | [中文](README.zh.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Tiếng Việt](README.vi.md) | [Français](README.fr.md) | [Italiano](README.it.md) | [Bahasa Indonesia](README.id.md) | [Malay](README.ms.md) | **Português**

# PicoClaw Fork 32-bit — Notas de Modificação e Documentação

Este documento descreve todas as modificações feitas neste fork (`picoclaw-edition`) em comparação com o projeto upstream [PicoClaw](https://github.com/sipeed/picoclaw).

---

## Índice

- [1. Suporte a Formatos Duplos de API Multimodal](#1-suporte-a-formatos-duplos-de-api-multimodal)
- [2. Compreensão de Vídeo](#2-compreensão-de-vídeo)
- [3. Mensagens de Vídeo no Telegram](#3-mensagens-de-vídeo-no-telegram)
- [4. Ferramenta load_video](#4-ferramenta-load_video)
- [5. Codificação Data URL de Áudio](#5-codificação-data-url-de-áudio)
- [6. Robustez de Configuração](#6-robustez-de-configuração)
- [7. Alterações na API](#7-alterações-na-api)
- [8. Suporte a Plataformas de 32 bits](#8-suporte-a-plataformas-de-32-bits)
- [Limitações Conhecidas](#limitações-conhecidas)

---

## 1. Suporte a Formatos Duplos de API Multimodal

**Problema:** O código upstream envia áudio no formato padrão OpenAI, mas a API do MiMo espera uma data URL completa.

**Solução:** Adicionada seleção de formato consciente do provedor.

**Comparação de formatos:**

| Tipo | Formato MiMo | Formato OpenAI Padrão |
|------|-------------|----------------------|
| Imagem | `image_url` + data URL | Idêntico (universal) |
| Áudio | `input_audio.data` = data URL completa | `input_audio.data` = base64 + campo `format` |
| Vídeo | `video_url` + `fps` + `media_resolution` | Sem tipo padrão |

---

## 2. Compreensão de Vídeo

**Alterações:** `pkg/agent/llm_media.go` — Função `describeVideoProxy()` adicionada com padrão de delegação.

---

## 3. Mensagens de Vídeo no Telegram

**Alterações:** `pkg/channels/telegram/telegram.go` — Tratamento de `msg.Video` adicionado.

---

## 4. Ferramenta load_video

**Novo recurso:** Ferramenta que permite ao AI carregar e analisar arquivos de vídeo locais.

---

## 5. Codificação Data URL de Áudio

**Alterações:** `pkg/agent/agent_media.go` — Áudio/vídeo em mensagens de usuário e resultados de ferramenta agora são codificados como data URLs.

---

## 6. Robustez de Configuração

- Campos de configuração desconhecidos agora são avisos, não erros
- Limite de corpo da requisição da API de configuração aumentado de 1MB para 20MB

---

## 7. Alterações na API

Documentação detalhada: [Referência da API](../api/README.pt-br.md)

---

## 8. Suporte a Plataformas de 32 bits

| SO | GOARCH | Nome do Binário |
|----|--------|-----------------|
| Linux | `386` | `picoclaw-linux-386` |
| Linux | `arm` | `picoclaw-linux-arm` |
| Windows | `386` | `picoclaw-windows-386.exe` |

---

## Limitações Conhecidas

- **Formato de vídeo:** `video_url` é exclusivo do MiMo.
- **API de chat:** Apenas texto. Entrada multimodal não suportada.

---

## Documentação Upstream

Documentação original do projeto PicoClaw:
- **English:** [../../UPSTREAM-README.md](../../UPSTREAM-README.md)
- **Português:** [UPSTREAM-README.pt-br.md](UPSTREAM-README.pt-br.md)
