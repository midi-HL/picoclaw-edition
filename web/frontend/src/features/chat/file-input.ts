import type { TFunction } from "i18next"
import { toast } from "sonner"

import type { ChatAttachment } from "@/store/chat"

// Generic (non-image) file attachments for the web chat composer.
// Images keep using image-input.ts (they are inlined as multimodal media);
// other files are sent as base64 payloads and written to the agent workspace,
// where the model reads them via read_file.

export const MAX_CHAT_FILE_SIZE_BYTES = 10 * 1024 * 1024
const MAX_CHAT_FILE_SIZE_LABEL = "10 MB"

function readFileAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      if (typeof reader.result === "string") {
        resolve(reader.result)
        return
      }
      reject(new Error("Failed to read file"))
    }
    reader.onerror = () =>
      reject(reader.error || new Error("Failed to read file"))
    reader.readAsDataURL(file)
  })
}

function isImageFile(file: File): boolean {
  return file.type.startsWith("image/")
}

/**
 * Builds generic-file attachments. Image files are skipped (they must go through
 * the image path so they can be inlined as multimodal media).
 */
export async function buildChatFileAttachments(
  files: readonly File[],
  t: TFunction,
): Promise<ChatAttachment[]> {
  const next: ChatAttachment[] = []

  for (const [index, file] of files.entries()) {
    const filename = file.name || `file-${index + 1}`

    if (isImageFile(file)) {
      // Silently skip; the caller routes images through buildChatImageAttachments.
      continue
    }

    if (file.size > MAX_CHAT_FILE_SIZE_BYTES) {
      toast.error(
        t("chat.fileTooLarge", {
          name: filename,
          size: MAX_CHAT_FILE_SIZE_LABEL,
        }),
      )
      continue
    }

    try {
      next.push({
        type: "file",
        filename,
        url: await readFileAsDataUrl(file),
        contentType: file.type || "application/octet-stream",
      })
    } catch {
      toast.error(
        t("chat.fileReadFailed", {
          name: filename,
        }),
      )
    }
  }

  return next
}
