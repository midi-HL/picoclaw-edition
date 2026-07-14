import { IconLoader2, IconTrash } from "@tabler/icons-react"
import { useCallback, useEffect, useState } from "react"

import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"

interface StickerItem {
  id: string
  source_type: string
  sticker_set_name?: string
  file_path: string
  telegram_file_id?: string
  emoji_hint?: string
  description: string
  usage_scenarios: string
  created_at: string
}

export function TelegramStickerManager() {
  const [stickers, setStickers] = useState<StickerItem[]>([])
  const [mode, setMode] = useState<"manual" | "import">("manual")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")

  // Mode A: Manual upload form state
  const [manualFile, setManualFile] = useState<File | null>(null)
  const [stickerId, setStickerId] = useState("")
  const [emojiHint, setEmojiHint] = useState("")
  const [description, setDescription] = useState("")
  const [scenarios, setScenarios] = useState("")

  // Mode B: Import form state
  const [setLink, setSetLink] = useState("")

  const fetchStickers = useCallback(async () => {
    try {
      const res = await fetch("/api/telegram/stickers")
      const data = await res.json()
      setStickers(data.stickers || [])
    } catch (e) {
      console.error("Failed to fetch stickers:", e)
    }
  }, [])

  useEffect(() => {
    fetchStickers()
  }, [fetchStickers])

  const handleManualSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!manualFile || !stickerId || !description || !scenarios) {
      setError("请填写所有必填项！")
      return
    }

    setLoading(true)
    setError("")

    const formData = new FormData()
    formData.append("file", manualFile)
    formData.append("id", stickerId)
    formData.append("emoji_hint", emojiHint)
    formData.append("description", description)
    formData.append("usage_scenarios", scenarios)

    try {
      const res = await fetch("/api/telegram/stickers/manual", {
        method: "POST",
        body: formData,
      })

      if (res.ok) {
        fetchStickers()
        // Reset form
        setManualFile(null)
        setStickerId("")
        setEmojiHint("")
        setDescription("")
        setScenarios("")
      } else {
        const err = await res.json()
        setError(err.error || "上传失败！")
      }
    } catch (e) {
      setError("上传失败：" + (e instanceof Error ? e.message : "未知错误"))
    } finally {
      setLoading(false)
    }
  }

  const handleImportSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!setLink) return

    setLoading(true)
    setError("")

    // Extract pack name from link
    let packName = setLink
    if (setLink.includes("addstickers/")) {
      packName = setLink.split("addstickers/")[1]
    }

    try {
      const res = await fetch("/api/telegram/stickers/import-set", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sticker_set_name: packName }),
      })

      if (res.ok) {
        fetchStickers()
        setSetLink("")
      } else {
        const err = await res.json()
        setError(err.error || "导入失败！")
      }
    } catch (e) {
      setError("导入失败：" + (e instanceof Error ? e.message : "未知错误"))
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm(`确定要删除表情包 ${id} 吗？`)) return

    try {
      await fetch(`/api/telegram/stickers/delete?id=${encodeURIComponent(id)}`, {
        method: "DELETE",
      })
      fetchStickers()
    } catch (e) {
      setError("删除失败：" + (e instanceof Error ? e.message : "未知错误"))
    }
  }

  return (
    <div className="space-y-6">
      <Card className="shadow-sm">
        <CardContent className="pt-6">
          <div className="bg-muted/50 rounded-lg p-4 text-sm">
            <p className="font-medium mb-2">功能说明</p>
            <ul className="space-y-1 text-muted-foreground">
              <li>
                1. <b>接收逻辑</b>：当用户向机器人发送表情时，如果是动画/视频表情（TGS/WebM），
                系统将自动降级提取静态缩略图转入大模型理解。
              </li>
              <li>
                2. <b>发送逻辑</b>：AI 通过输出 [SEND_STICKER: ID] 标记自主决定发送哪张表情包，
                此标记会在发送前被系统截断。
              </li>
            </ul>
          </div>
        </CardContent>
      </Card>

      <Card className="shadow-sm">
        <CardContent className="pt-6 space-y-4">
          <Label className="text-base font-bold">新增表情包</Label>

          <div className="flex gap-2">
            <Button
              type="button"
              variant={mode === "manual" ? "default" : "outline"}
              onClick={() => setMode("manual")}
            >
              模式 A: 本地手动上传
            </Button>
            <Button
              type="button"
              variant={mode === "import" ? "default" : "outline"}
              onClick={() => setMode("import")}
            >
              模式 B: TG 官方套图导入
            </Button>
          </div>

          {mode === "manual" ? (
            <form onSubmit={handleManualSubmit} className="space-y-4 pt-2">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>
                    StickerID <span className="text-destructive">*</span>
                  </Label>
                  <Input
                    value={stickerId}
                    onChange={(e) => setStickerId(e.target.value)}
                    placeholder="如: happy_dog"
                    required
                  />
                </div>
                <div className="space-y-2">
                  <Label>关联快捷 Emoji (选填)</Label>
                  <Input
                    value={emojiHint}
                    onChange={(e) => setEmojiHint(e.target.value)}
                    placeholder="如: 😄"
                  />
                </div>
              </div>

              <div className="space-y-2">
                <Label>
                  上传图片文件 <span className="text-destructive">*</span>
                </Label>
                <Input
                  type="file"
                  onChange={(e) => setManualFile(e.target.files?.[0] || null)}
                  accept="image/*"
                  required
                />
              </div>

              <div className="space-y-2">
                <Label>
                  画面描述 <span className="text-destructive">*</span>
                </Label>
                <Textarea
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="描述例如: 一只穿着宇航服的小猫在太空中开心地挥手..."
                  required
                />
              </div>

              <div className="space-y-2">
                <Label>
                  适用聊天场景 <span className="text-destructive">*</span>
                </Label>
                <Textarea
                  value={scenarios}
                  onChange={(e) => setScenarios(e.target.value)}
                  placeholder="场景例如: 当用户表达对外太空的喜爱，或者氛围愉快时使用..."
                  required
                />
              </div>

              <Button type="submit" disabled={loading}>
                {loading ? (
                  <>
                    <IconLoader2 className="mr-2 h-4 w-4 animate-spin" />
                    上传中...
                  </>
                ) : (
                  "确认并录入表情"
                )}
              </Button>
            </form>
          ) : (
            <form onSubmit={handleImportSubmit} className="space-y-4 pt-2">
              <div className="space-y-2">
                <Label>
                  Telegram 贴纸集链接或包名{" "}
                  <span className="text-destructive">*</span>
                </Label>
                <Input
                  value={setLink}
                  onChange={(e) => setSetLink(e.target.value)}
                  placeholder="https://t.me/addstickers/LovelyPanda 或 LovelyPanda"
                  required
                />
              </div>

              <Button type="submit" disabled={loading} variant="secondary">
                {loading ? (
                  <>
                    <IconLoader2 className="mr-2 h-4 w-4 animate-spin" />
                    后台正在多模态获取并智能提取中...
                  </>
                ) : (
                  "一键自动导入贴纸包"
                )}
              </Button>
            </form>
          )}

          {error && (
            <div className="text-destructive text-sm bg-destructive/10 p-3 rounded-lg">
              {error}
            </div>
          )}
        </CardContent>
      </Card>

      <div>
        <Label className="text-base font-bold block mb-3">
          当前已注册表情包列表 ({stickers.length})
        </Label>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {stickers.map((item) => (
            <Card key={item.id} className="overflow-hidden">
              <div className="h-40 bg-muted flex items-center justify-center p-2">
                {item.file_path ? (
                  <img
                    src={`/api/media/file?path=${encodeURIComponent(item.file_path)}`}
                    alt={item.id}
                    className="max-h-full max-w-full object-contain"
                    onError={(e) => {
                      ;(e.target as HTMLImageElement).style.display = "none"
                    }}
                  />
                ) : (
                  <span className="text-muted-foreground text-sm">无预览</span>
                )}
              </div>
              <CardContent className="p-3 space-y-2 text-sm">
                <div className="flex justify-between items-center">
                  <span className="font-bold text-primary">{item.id}</span>
                  <span className="bg-secondary px-2 py-0.5 rounded text-xs">
                    {item.source_type === "manual"
                      ? "手动上传"
                      : `${item.sticker_set_name || "导入"} 导入`}
                  </span>
                </div>
                {item.emoji_hint && (
                  <div>
                    <b>Emoji:</b> {item.emoji_hint}
                  </div>
                )}
                <div className="text-muted-foreground line-clamp-2">
                  <b>描述:</b> {item.description}
                </div>
                <div className="text-muted-foreground line-clamp-2">
                  <b>场景:</b> {item.usage_scenarios}
                </div>
                <div className="pt-2 flex justify-end">
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => handleDelete(item.id)}
                  >
                    <IconTrash className="h-4 w-4" />
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    </div>
  )
}
