import { useCallback, useEffect, useRef, useState } from "react"
import { Check, CircleHelp, FileImage, Loader2, Paperclip, RefreshCw, Send, X } from "lucide-react"
import { organizerApi, OrganizerApiError, type AssistantConversationView, type AssistantPlanRecord, type AssistantTurn, type Candidate } from "@/lib/organizer-api"

function errorMessage(error: unknown) { return error instanceof OrganizerApiError || error instanceof Error ? error.message : "发生未知错误" }
function makeKey() { return `agenda-${Date.now()}-${Math.random().toString(36).slice(2)}` }
const maxAttachments = 10

function assistantContent(turn: AssistantTurn) {
  const value = turn.structuredResult
  if (!value || typeof value !== "object") return null
  const result = value as { kind?: string; reply?: string; ambiguityQuestions?: string[] }
  return result
}
function queryResults(record?: AssistantPlanRecord) {
  if (!record) return null
  const action = record.actions.find((value) => value.operation === "list_items" && value.result && typeof value.result === "object")
  if (!action) return null
  const result = action.result as { count?: number; items?: { title?: string; status?: string; project?: string; dueAt?: string }[] }
  const items = Array.isArray(result.items) ? result.items : []
  return <div className="mt-2 border-l-2 border-primary/30 pl-3 text-sm"><p>找到 {result.count ?? items.length} 项</p><ul className="mt-1 space-y-1">{items.slice(0, 30).map((item, index) => <li key={`${item.title ?? "item"}-${index}`}><span className="font-medium">{item.title ?? "未命名事项"}</span><span className="ml-2 text-muted-foreground">{item.status}{item.project ? ` · ${item.project}` : ""}{item.dueAt ? ` · 截止 ${new Date(item.dueAt).toLocaleString("zh-CN")}` : ""}</span></li>)}</ul>{items.length < (result.count ?? items.length) ? <p className="mt-1 text-xs text-muted-foreground">仅显示前 {items.length} 项</p> : null}</div>
}

export function AssistantPanel({ embedded = false, onClose, onChanged }: { embedded?: boolean; onClose?: () => void; onChanged: () => Promise<void> }) {
  const [conversation, setConversation] = useState<AssistantConversationView | null>(null)
  const [composer, setComposer] = useState("")
  const [attachments, setAttachments] = useState<File[]>([])
  const [dragging, setDragging] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [pending, setPending] = useState<{ plan?: AssistantPlanRecord; receipt?: AssistantPlanRecord; clarification?: string[] }>({})
  const endRef = useRef<HTMLDivElement>(null)
  const conversationKey = "agenda-assistant-conversation"
  const fileRef = useRef<HTMLInputElement>(null)

  const load = useCallback(async () => {
    let id = window.localStorage.getItem(conversationKey)
    if (!id) { const created = await organizerApi.createAssistantConversation(); id = created.id; window.localStorage.setItem(conversationKey, id) }
    try { setConversation(await organizerApi.getAssistantConversation(id)) }
    catch (caught) {
      if (caught instanceof OrganizerApiError && caught.status === 404) {
        window.localStorage.removeItem(conversationKey)
        const created = await organizerApi.createAssistantConversation()
        window.localStorage.setItem(conversationKey, created.id)
        setConversation(await organizerApi.getAssistantConversation(created.id))
      } else throw caught
    }
  }, [])
  useEffect(() => { void load().catch((caught) => setError(errorMessage(caught))) }, [load])
  useEffect(() => { endRef.current?.scrollIntoView({ behavior: "smooth" }) }, [conversation?.turns?.length, busy])

  async function submit(event?: React.FormEvent) {
    event?.preventDefault()
    const text = composer.trim()
    if ((!text && !attachments.length) || busy || !conversation) return
    setBusy(true); setError(""); setPending({})
    try {
      if (attachments.length) {
        const images = attachments
        const capture = await organizerApi.createCapture(text, images)
        const parsed = await organizerApi.parseCapture(capture.id)
        setComposer(""); setAttachments([])
        await onChanged()
        setConversation((current) => current ? { ...current, turns: [...current.turns, { id: `image-${Date.now()}`, role: "assistant", content: imageResultMessage(images, parsed.items), structuredResult: { kind: "image_capture", captureId: capture.id, candidates: parsed.items } }] } : current)
        return
      }
      const response = await organizerApi.submitAssistantTurn(conversation.id, text, makeKey())
      setComposer("")
      setPending({ plan: response.plan, receipt: response.receipt, clarification: response.clarificationQuestions })
      setConversation(await organizerApi.getAssistantConversation(conversation.id))
      await onChanged()
    } catch (caught) { setError(`发送失败：${errorMessage(caught)}`) }
    finally { setBusy(false) }
  }

  function chooseFiles(files: Iterable<File>) {
    const selected = Array.from(files)
    if (!selected.length) return
    const supportedTypes = new Set(["image/png", "image/jpeg", "image/webp", "image/gif"])
    if (selected.some((file) => !supportedTypes.has(file.type.toLowerCase()))) { setError("请选择 PNG、JPEG、WebP 或 GIF 图片"); return }
    const additions = selected.filter((file) => !attachments.some((value) => value.name === file.name && value.size === file.size && value.lastModified === file.lastModified))
    setError(attachments.length + additions.length > maxAttachments ? `一次最多添加 ${maxAttachments} 张图片` : "")
    setAttachments((current) => {
      const next = [...current]
      for (const file of selected) {
        if (!next.some((value) => value.name === file.name && value.size === file.size && value.lastModified === file.lastModified)) next.push(file)
      }
      return next.slice(0, maxAttachments)
    })
  }

  function imageResultMessage(files: File[], items: Candidate[]) {
    const label = files.length === 1 ? `图片「${files[0].name}」` : `${files.length} 张图片`
    if (!items.length) return `已收到${label}，但没有识别出明确事项。原图已保存到待整理。`
    return `已识别${label}，发现 ${items.length} 个事项，已放入待整理。`
  }

  async function act(action: "confirm" | "cancel") {
    if (!pending.plan || busy) return
    setBusy(true); setError("")
    try {
      const response = action === "confirm" ? await organizerApi.confirmAssistantPlan(pending.plan.id) : await organizerApi.cancelAssistantPlan(pending.plan.id)
      setConversation(await organizerApi.getAssistantConversation(pending.plan.conversationId ?? conversation!.id))
      setPending({ plan: response.plan, receipt: response.receipt, clarification: response.clarificationQuestions })
      await onChanged()
    } catch (caught) { setError(`操作失败：${errorMessage(caught)}`) }
    finally { setBusy(false) }
  }

  async function undo() {
    if (!pending.plan || busy) return
    setBusy(true); setError("")
    try {
      const response = await organizerApi.undoAssistantPlan(pending.plan.id)
      setPending({ plan: response.plan, receipt: response.receipt, clarification: response.clarificationQuestions })
      await load(); await onChanged()
    } catch (caught) { setError(`撤销失败：${errorMessage(caught)}`) }
    finally { setBusy(false) }
  }

  const turns = conversation?.turns ?? []
  return <section className={embedded ? "border-b border-border bg-card px-4 py-4 sm:px-6" : "fixed inset-x-0 bottom-0 z-40 flex max-h-[88vh] flex-col border-t border-border bg-background shadow-2xl sm:inset-auto sm:bottom-5 sm:right-5 sm:w-[min(440px,calc(100vw-2rem))] sm:border"}>
    {!embedded ? <header className="flex items-center justify-between border-b border-border bg-card px-4 py-3"><div className="flex items-center gap-2"><CircleHelp className="h-4 w-4 text-primary" /><h2 className="text-sm font-semibold">Agenda AI</h2></div><button onClick={onClose} className="p-1.5 hover:bg-muted" aria-label="关闭"><X className="h-4 w-4" /></button></header> : <div className="mx-auto mb-3 flex max-w-5xl items-center gap-2"><CircleHelp className="h-4 w-4 text-primary" /><h2 className="text-sm font-semibold">Agenda AI</h2></div>}
    <div className={embedded ? "mx-auto max-h-72 w-full max-w-5xl space-y-3 overflow-y-auto pb-3" : "min-h-0 flex-1 space-y-3 overflow-y-auto p-3"}>
      {!turns.length && !busy ? <p className="py-3 text-sm text-muted-foreground">输入日程、任务、项目内容，也可以直接问我问题。</p> : null}
      {turns.map((turn) => {
        const structured = assistantContent(turn)
        const questions = structured?.ambiguityQuestions ?? []
        const receipt = turn.role === "action" && turn.structuredResult && typeof turn.structuredResult === "object" ? turn.structuredResult as AssistantPlanRecord : undefined
        return <div key={turn.id} className={turn.role === "user" ? "ml-8 border border-primary/20 bg-primary/5 px-3 py-2 text-sm whitespace-pre-wrap" : "mr-8 border border-border bg-background px-3 py-3 text-sm"}>
          <p className="whitespace-pre-wrap">{turn.content}</p>
          {questions.length ? <ul className="mt-2 list-disc pl-5 text-amber-700 dark:text-amber-300">{questions.map((question) => <li key={question}>{question}</li>)}</ul> : null}
          {receipt ? <>{receipt.actions.map((action) => <p key={action.actionId} className="mt-2 text-xs text-muted-foreground">{action.operation}：{action.state}{action.error ? ` · ${action.error}` : ""}</p>)}{queryResults(receipt)}</> : null}
        </div>
      })}
      {(pending.plan || pending.receipt || pending.clarification?.length) ? <div className="mr-8 border border-border bg-background px-3 py-3 text-sm"><p>{pending.plan?.plan.intentSummary ?? pending.receipt?.plan.intentSummary ?? "需要补充信息"}</p>{pending.clarification?.map((question) => <p key={question} className="mt-2 text-amber-700 dark:text-amber-300">{question}</p>)}{(pending.receipt ?? pending.plan)?.actions.map((action) => <p key={action.actionId} className="mt-2 text-xs text-muted-foreground">{action.operation}：{action.state}{action.error ? ` · ${action.error}` : ""}</p>)}{queryResults(pending.receipt ?? pending.plan)}{pending.receipt?.state === "completed" ? <button onClick={() => void undo()} className="mt-3 border border-border px-2.5 py-1.5 text-xs hover:bg-muted">撤销这次修改</button> : null}</div> : null}
      {pending.plan?.requiresConfirmation && pending.plan.state === "awaiting_confirmation" ? <div className="flex gap-2 pl-2"><button disabled={busy} onClick={() => void act("confirm")} className="flex items-center gap-1.5 bg-primary px-3 py-2 text-xs font-medium text-primary-foreground disabled:opacity-50"><Check className="h-3.5 w-3.5" />确认执行</button><button disabled={busy} onClick={() => void act("cancel")} className="flex items-center gap-1.5 border border-border px-3 py-2 text-xs disabled:opacity-50"><X className="h-3.5 w-3.5" />取消</button></div> : null}
      {error ? <div className="flex items-center justify-between gap-2 border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive"><span>{error}</span><button onClick={() => void submit()} disabled={busy || (!composer.trim() && !attachments.length)} aria-label="重试"><RefreshCw className="h-3.5 w-3.5" /></button></div> : null}<div ref={endRef} />
    </div>
    <form onSubmit={(event) => void submit(event)} className={embedded ? "mx-auto w-full max-w-5xl border-t border-border pt-3" : "border-t border-border bg-card p-3"} onDragOver={(event) => { event.preventDefault(); setDragging(true) }} onDragLeave={() => setDragging(false)} onDrop={(event) => { event.preventDefault(); setDragging(false); chooseFiles(event.dataTransfer.files) }} onPaste={(event) => { const files = Array.from(event.clipboardData.files).filter((value) => value.type.startsWith("image/")); if (files.length) { event.preventDefault(); chooseFiles(files) } }}>
      <div className={`relative ${dragging ? "ring-2 ring-primary/50" : ""}`}>
        <textarea value={composer} onChange={(event) => setComposer(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) { event.preventDefault(); void submit() } }} rows={2} disabled={busy} placeholder="输入任何想安排、调整、查询或讨论的内容…也可以拖入图片" className="w-full resize-y border border-border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/30" aria-label="发送给 Agenda AI" />
        {dragging ? <div className="pointer-events-none absolute inset-0 flex items-center justify-center bg-primary/10 text-sm font-medium text-primary">松开即可添加图片</div> : null}
      </div>
      {attachments.length ? <div className="mt-2 grid gap-1.5 sm:grid-cols-2">{attachments.map((attachment, index) => <div key={`${attachment.name}-${attachment.size}-${attachment.lastModified}`} className="flex min-w-0 items-center gap-2 border border-primary/20 bg-primary/5 px-2.5 py-2 text-xs"><FileImage className="h-4 w-4 shrink-0 text-primary" /><span className="min-w-0 flex-1 truncate">{attachment.name}</span><button type="button" onClick={() => setAttachments((current) => current.filter((_, valueIndex) => valueIndex !== index))} aria-label={`移除图片 ${attachment.name}`} className="p-1 hover:bg-muted"><X className="h-3.5 w-3.5" /></button></div>)}</div> : null}
      <div className="mt-2 flex items-center justify-between gap-2"><span className="text-[11px] text-muted-foreground">可选择、拖入或粘贴多张图片（最多 {maxAttachments} 张）；输入法语音转文字也可直接使用</span><div className="flex items-center gap-2"><input ref={fileRef} type="file" multiple accept="image/png,image/jpeg,image/webp,image/gif" className="hidden" onChange={(event) => { chooseFiles(event.target.files ?? []); event.currentTarget.value = "" }} /><button type="button" onClick={() => fileRef.current?.click()} disabled={busy || attachments.length >= maxAttachments} aria-label="添加图片" title="添加图片" className="border border-border p-2 hover:bg-muted disabled:opacity-50"><Paperclip className="h-4 w-4" /></button><button type="submit" disabled={busy || (!composer.trim() && !attachments.length)} className="flex items-center gap-1.5 bg-primary px-3 py-2 text-xs font-medium text-primary-foreground disabled:opacity-50">{busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}{busy ? "处理中" : "发送"}</button></div></div>
    </form>
  </section>
}
