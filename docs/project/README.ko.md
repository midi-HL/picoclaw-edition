[English](../../README.md) | [中文](README.zh.md) | [日本語](README.ja.md) | [Português](README.pt-br.md) | [Tiếng Việt](README.vi.md) | [Français](README.fr.md) | [Italiano](README.it.md) | [Bahasa Indonesia](README.id.md) | [Malay](README.ms.md) | **한국어**

# PicoClaw 32비트 Fork — 변경 사항 및 문서

이 문서는 이 fork(`picoclaw-edition`)에서 상류 [PicoClaw](https://github.com/sipeed/picoclaw) 프로젝트 대비 모든 변경 사항을 설명합니다.

---

## 목차

- [1. 듀얼 멀티모달 API 형식 지원](#1-듀얼-멀티모달-api-형식-지원)
- [2. 비디오 이해](#2-비디오-이해)
- [3. Telegram 비디오 메시지](#3-telegram-비디오-메시지)
- [4. load_video 도구](#4-load_video-도구)
- [5. 오디오 Data URL 인코딩](#5-오디오-data-url-인코딩)
- [6. 설정 견고성](#6-설정-견고성)
- [7. API 변경](#7-api-변경)
- [8. 32비트 플랫폼 지원](#8-32비트-플랫폼-지원)
- [알려진 제한 사항](#알려진-제한-사항)

---

## 1. 듀얼 멀티모달 API 형식 지원

**문제:** 상류 프로바이더 코드는 표준 OpenAI 형식으로 오디오를 전송하지만, MiMo API는 완전한 data URL을 요구합니다.

**해결책:** 프로바이더 인식 형식 선택을 추가하여 대상 프로바이더를 자동 감지하고 적절한 형식을 전송합니다.

**형식 비교:**

| 유형 | MiMo 형식 | 표준 OpenAI 형식 |
|------|----------|-----------------|
| 이미지 | `image_url` + data URL | 동일 (범용) |
| 오디오 | `input_audio.data` = 완전한 data URL | `input_audio.data` = base64 + `format` 필드 |
| 비디오 | `video_url` + `fps` + `media_resolution` | 표준 유형 없음 |

---

## 2. 비디오 이해

**변경 사항:**
- `pkg/agent/llm_media.go` — `describeVideoProxy()` 함수 추가, **위임 패턴** 구현

---

## 3. Telegram 비디오 메시지

**변경 사항:** `pkg/channels/telegram/telegram.go` — `msg.Video` 처리 추가.

---

## 4. load_video 도구

**새 기능:** AI가 로컬 비디오 파일을 로드하고 분석할 수 있는 도구.

---

## 5. 오디오 Data URL 인코딩

**변경 사항:** `pkg/agent/agent_media.go` — 사용자 메시지와 도구 결과의 오디오/비디오를 data URL로 인코딩.

---

## 6. 설정 견고성

- 알 수 없는 설정 필드가 오류 대신 경고로 변경
- 설정 API 요청 본문 제한 1MB → 20MB로 증가

---

## 7. API 변경

상세: [API 레퍼런스](../api/README.ko.md)

---

## 8. 32비트 플랫폼 지원

| OS | GOARCH | 바이너리 이름 |
|----|--------|-------------|
| Linux | `386` | `picoclaw-linux-386` |
| Linux | `arm` | `picoclaw-linux-arm` |
| Windows | `386` | `picoclaw-windows-386.exe` |

---

## 알려진 제한 사항

- **비디오 형식:** `video_url`는 MiMo 전용. 비-MiMo 프로바이더에서는 비디오가 건너뛰어집니다.
- **채팅 API:** 텍스트만 지원. 멀티모달 입력 미지원.

---

## 상류 문서

원본 PicoClaw 프로젝트 문서:
- **English:** [../../UPSTREAM-README.md](../../UPSTREAM-README.md)
- **한국어:** [UPSTREAM-README.ko.md](UPSTREAM-README.ko.md)
