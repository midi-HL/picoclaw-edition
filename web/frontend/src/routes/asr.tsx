import { createFileRoute } from "@tanstack/react-router"

import { ASRPage } from "@/components/asr/asr-page"

export const Route = createFileRoute("/asr")({
  component: ASRPage,
})
