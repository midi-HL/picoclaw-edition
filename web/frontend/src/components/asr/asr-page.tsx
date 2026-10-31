import {
  IconAlertTriangle,
  IconCheck,
  IconDownload,
  IconMicrophone,
  IconPlayerPlay,
  IconRefresh,
  IconTrash,
} from "@tabler/icons-react"
import { useCallback, useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"

interface ASRModel {
  id: string
  name: string
  size_mb: number
  speed: string
  accuracy: string
  memory: string
  languages: string
  model_type: string
  recommended: boolean
  description: string
  downloaded: boolean
  active: boolean
}

interface ASRStatus {
  platform_supported: boolean
  platform?: string
  installed: boolean
  binary_path: string
  models_dir: string
  downloaded: string[]
  active?: boolean
  asr_source?: string
  config?: {
    enabled: boolean
    model_id?: string
    threads?: number
  }
}

interface ProgressState {
  stage: string
  message?: string
  percent?: number
  bytes_done?: number
  bytes_total?: number
  error?: string
}

const IDLE: ProgressState = { stage: "idle" }

export function ASRPage() {
  const { t } = useTranslation()
  const [status, setStatus] = useState<ASRStatus | null>(null)
  const [models, setModels] = useState<ASRModel[]>([])
  const [loading, setLoading] = useState(true)
  const [installProgress, setInstallProgress] =
    useState<ProgressState>(IDLE)
  const [modelProgress, setModelProgress] = useState<
    Record<string, ProgressState>
  >({})
  const [threads, setThreads] = useState(0)
  const [testText, setTestText] = useState("")
  const [testing, setTesting] = useState(false)
  const pollTimers = useRef<Record<string, number>>({})

  const refresh = useCallback(async () => {
    try {
      const [s, m] = await Promise.all([
        fetch("/api/asr/local/status").then((r) => r.json()),
        fetch("/api/asr/local/models").then((r) => r.json()),
      ])
      setStatus(s)
      setModels(m.models ?? [])
      if (s.config?.threads) setThreads(s.config.threads)
    } catch (err) {
      toast.error(t("asr.loadFailed"))
      console.error(err)
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => {
    void refresh()
    return () => {
      Object.values(pollTimers.current).forEach((id) => window.clearInterval(id))
    }
  }, [refresh])

  const poll = useCallback(
    (task: string, onUpdate: (p: ProgressState) => void) => {
      const existing = pollTimers.current[task]
      if (existing) window.clearInterval(existing)

      const id = window.setInterval(async () => {
        try {
          const p: ProgressState = await fetch(
            `/api/asr/local/progress?task=${encodeURIComponent(task)}`,
          ).then((r) => r.json())
          onUpdate(p)
          if (p.stage === "done" || p.stage === "error" || p.stage === "idle") {
            window.clearInterval(id)
            delete pollTimers.current[task]
            if (p.stage === "done") {
              toast.success(t("asr.completed"))
              void refresh()
            } else if (p.stage === "error") {
              toast.error(p.error || t("asr.failed"))
            }
          }
        } catch {
          // keep polling; transient network errors are fine
        }
      }, 800)
      pollTimers.current[task] = id
    },
    [refresh, t],
  )

  const installBinary = async () => {
    setInstallProgress({ stage: "downloading", message: t("asr.preparing") })
    poll("binary", setInstallProgress)
    const res = await fetch("/api/asr/local/install", { method: "POST" })
    if (!res.ok) {
      const msg = await res.text()
      toast.error(msg || t("asr.failed"))
      setInstallProgress({ stage: "error", error: msg })
    }
  }

  const downloadModel = async (id: string) => {
    setModelProgress((p) => ({ ...p, [id]: { stage: "downloading" } }))
    poll(`model:${id}`, (prog) =>
      setModelProgress((p) => ({ ...p, [id]: prog })),
    )
    const res = await fetch("/api/asr/local/download-model", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ model_id: id }),
    })
    if (!res.ok) {
      const msg = await res.text()
      toast.error(msg || t("asr.failed"))
      setModelProgress((p) => ({ ...p, [id]: { stage: "error", error: msg } }))
    }
  }

  const enableModel = async (id: string, enabled: boolean) => {
    const res = await fetch("/api/asr/local/enable", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ model_id: id, enabled, threads }),
    })
    if (!res.ok) {
      toast.error(await res.text())
      return
    }
    toast.success(enabled ? t("asr.enabled") : t("asr.disabled"))
    void refresh()
  }

  const removeModel = async (id: string) => {
    const res = await fetch("/api/asr/local/uninstall", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ model_id: id }),
    })
    if (!res.ok) {
      toast.error(await res.text())
      return
    }
    toast.success(t("asr.removed"))
    void refresh()
  }

  const runTest = async (file: File) => {
    setTesting(true)
    setTestText("")
    try {
      const form = new FormData()
      form.append("audio", file)
      const res = await fetch("/api/asr/local/test", {
        method: "POST",
        body: form,
      })
      if (!res.ok) {
        toast.error(await res.text())
        return
      }
      const data = await res.json()
      setTestText(data.text ?? "")
      toast.success(`${t("asr.testDone")} ${data.elapsed_ms}ms`)
    } catch (err) {
      toast.error(String(err))
    } finally {
      setTesting(false)
    }
  }

  const activeModel = models.find((m) => m.active)

  return (
    <div className="bg-background/95 flex h-full flex-col">
      <PageHeader
        title={t("navigation.asr")}
        titleExtra={
          <Button variant="outline" size="sm" onClick={() => void refresh()}>
            <IconRefresh className="mr-1.5 size-4" />
            {t("asr.refresh")}
          </Button>
        }
      />

      <div className="flex-1 overflow-y-auto p-4">
        <div className="mx-auto max-w-4xl space-y-4">
          {loading && <p className="text-muted-foreground">{t("asr.loading")}</p>}

          {status && !status.platform_supported && (
            <Card>
              <CardContent className="flex items-start gap-3 p-4">
                <IconAlertTriangle className="mt-0.5 size-5 text-amber-500" />
                <div>
                  <p className="font-medium">{t("asr.unsupportedTitle")}</p>
                  <p className="text-muted-foreground text-sm">
                    {t("asr.unsupportedBody", { platform: status.platform })}
                  </p>
                </div>
              </CardContent>
            </Card>
          )}

          {status?.platform_supported && (
            <>
              {/* Runtime install */}
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2 text-base">
                    <IconMicrophone className="size-4" />
                    {t("asr.runtimeTitle")}
                    {status.installed ? (
                      <Badge variant="secondary">
                        <IconCheck className="mr-1 size-3" />
                        {t("asr.installed")}
                      </Badge>
                    ) : (
                      <Badge variant="outline">{t("asr.notInstalled")}</Badge>
                    )}
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  <p className="text-muted-foreground text-sm">
                    {t("asr.runtimeHint")}
                  </p>
                  <p className="text-muted-foreground font-mono text-xs break-all">
                    {status.binary_path}
                  </p>

                  {!status.installed && (
                    <Button
                      onClick={() => void installBinary()}
                      disabled={
                        installProgress.stage === "downloading" ||
                        installProgress.stage === "extracting"
                      }
                    >
                      <IconDownload className="mr-1.5 size-4" />
                      {t("asr.installRuntime")}
                    </Button>
                  )}

                  {installProgress.stage !== "idle" && (
                    <ProgressBlock progress={installProgress} />
                  )}
                </CardContent>
              </Card>

              {/* Models */}
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">
                    {t("asr.modelsTitle")}
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  <p className="text-muted-foreground text-sm">
                    {t("asr.modelsHint")}
                  </p>

                  <div className="grid gap-3 md:grid-cols-3">
                    {models.map((m) => (
                      <ModelCard
                        key={m.id}
                        model={m}
                        progress={modelProgress[m.id] ?? IDLE}
                        runtimeInstalled={status.installed}
                        onDownload={() => void downloadModel(m.id)}
                        onEnable={() => void enableModel(m.id, true)}
                        onDisable={() => void enableModel(m.id, false)}
                        onRemove={() => void removeModel(m.id)}
                      />
                    ))}
                  </div>
                </CardContent>
              </Card>

              {/* Active config + test */}
              <Card>
                <CardHeader>
                  <CardTitle className="text-base">
                    {t("asr.testTitle")}
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  {!activeModel ? (
                    <p className="text-muted-foreground text-sm">
                      {t("asr.testNeedModel")}
                    </p>
                  ) : (
                    <>
                      <div className="flex items-center gap-3">
                        <span className="text-sm">
                          {t("asr.threads")}
                        </span>
                        <Input
                          type="number"
                          min={0}
                          max={32}
                          value={threads}
                          onChange={(e) =>
                            setThreads(Number(e.target.value) || 0)
                          }
                          className="w-24"
                        />
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() =>
                            void enableModel(activeModel.id, true)
                          }
                        >
                          {t("asr.save")}
                        </Button>
                      </div>

                      <label className="inline-flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm">
                        <IconPlayerPlay className="size-4" />
                        {testing ? t("asr.testing") : t("asr.testUpload")}
                        <input
                          type="file"
                          accept="audio/*,.wav,.mp3,.m4a,.ogg,.flac"
                          className="hidden"
                          onChange={(e) => {
                            const f = e.target.files?.[0]
                            e.target.value = ""
                            if (f) void runTest(f)
                          }}
                        />
                      </label>

                      {testText && (
                        <div className="rounded-md border bg-muted/40 p-3 text-sm whitespace-pre-wrap">
                          {testText}
                        </div>
                      )}
                    </>
                  )}
                </CardContent>
              </Card>
            </>
          )}
        </div>
      </div>
    </div>
  )
}


function Progress({ value }: { value: number }) {
  const pct = Math.max(0, Math.min(100, value))
  return (
    <div className="bg-secondary h-2 w-full overflow-hidden rounded-full">
      <div
        className="bg-primary h-full rounded-full transition-all duration-200"
        style={{ width: `${pct}%` }}
      />
    </div>
  )
}

function ProgressBlock({ progress }: { progress: ProgressState }) {
  const { t } = useTranslation()
  const pct = Math.max(0, Math.min(100, progress.percent ?? 0))
  return (
    <div className="space-y-1">
      <Progress value={pct} />
      <p className="text-muted-foreground text-xs">
        {progress.message || progress.stage}
        {progress.stage === "error" && progress.error
          ? ` — ${progress.error}`
          : ""}
        {progress.stage === "done" ? ` — ${t("asr.completed")}` : ""}
      </p>
    </div>
  )
}

function ModelCard({
  model,
  progress,
  runtimeInstalled,
  onDownload,
  onEnable,
  onDisable,
  onRemove,
}: {
  model: ASRModel
  progress: ProgressState
  runtimeInstalled: boolean
  onDownload: () => void
  onEnable: () => void
  onDisable: () => void
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const busy =
    progress.stage === "downloading" || progress.stage === "extracting"

  return (
    <div
      className={`flex flex-col gap-2 rounded-lg border p-3 ${
        model.active ? "ring-2 ring-primary" : ""
      }`}
    >
      <div className="flex items-start justify-between gap-2">
        <div>
          <p className="text-sm font-semibold">{model.name}</p>
          <p className="text-muted-foreground text-xs">{model.size_mb} MB</p>
        </div>
        {model.recommended && (
          <Badge variant="secondary" className="shrink-0 text-[10px]">
            {t("asr.recommended")}
          </Badge>
        )}
      </div>

      <dl className="text-muted-foreground space-y-0.5 text-xs">
        <div>
          <dt className="inline font-medium">{t("asr.speed")}：</dt>
          <dd className="inline">{model.speed}</dd>
        </div>
        <div>
          <dt className="inline font-medium">{t("asr.accuracy")}：</dt>
          <dd className="inline">{model.accuracy}</dd>
        </div>
        <div>
          <dt className="inline font-medium">{t("asr.memory")}：</dt>
          <dd className="inline">{model.memory}</dd>
        </div>
        <div>
          <dt className="inline font-medium">{t("asr.languages")}：</dt>
          <dd className="inline">{model.languages}</dd>
        </div>
      </dl>

      <p className="text-muted-foreground line-clamp-2 text-xs">
        {model.description}
      </p>

      {busy && <Progress value={Math.max(0, Math.min(100, progress.percent ?? 0))} />}
      {progress.stage === "error" && (
        <p className="text-destructive text-xs">{progress.error}</p>
      )}

      <div className="mt-auto flex flex-wrap gap-1.5 pt-1">
        {!model.downloaded && (
          <Button
            size="sm"
            variant="outline"
            onClick={onDownload}
            disabled={busy || !runtimeInstalled}
          >
            <IconDownload className="mr-1 size-3.5" />
            {busy ? t("asr.downloading") : t("asr.download")}
          </Button>
        )}

        {model.downloaded && !model.active && (
          <Button size="sm" onClick={onEnable}>
            {t("asr.enable")}
          </Button>
        )}

        {model.active && (
          <Button size="sm" variant="secondary" onClick={onDisable}>
            <IconCheck className="mr-1 size-3.5" />
            {t("asr.enabled")}
          </Button>
        )}

        {model.downloaded && (
          <Button size="sm" variant="ghost" onClick={onRemove}>
            <IconTrash className="size-3.5" />
          </Button>
        )}
      </div>
    </div>
  )
}
