import { useCallback, useEffect, useRef, useState } from "react"
import { Check, CircleHelp, FileImage, Loader2, Paperclip, Plus, RefreshCw, Send, X } from "lucide-react"
import { organizerApi, OrganizerApiError, type AssistantConversation, type AssistantConversationView, type AssistantPlanRecord, type AssistantTurn } from "@/lib/organizer-api"

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

function actionLabel(operation: string) {
  const labels: Record<string, string> = { create_item: "新增事项", update_item: "调整事项", complete_item: "完成事项", reopen_item: "重新打开", archive_item: "归档事项", set_reminder: "设置提醒", create_project: "新建项目", update_project: "调整项目", merge_projects: "合并项目", move_items_to_project: "移动到项目", list_items: "查询事项" }
  return labels[operation] ?? operation
}

function planPreview(record?: AssistantPlanRecord) {
  if (!record?.plan?.actions?.length) return null
  return <div className="mt-3 space-y-1.5">{record.plan.actions.map((action) => <div key={action.actionId} className="flex items-center gap-2 text-xs"><span className="h-1.5 w-1.5 rounded-full bg-primary" /><span>{actionLabel(action.operation)}</span><span className="text-muted-foreground">{action.state === "pending" ? "等待确认" : action.state}</span></div>)}</div>
}

export function AssistantPanel({ embedded = false, onClose, onChanged }: { embedded?: boolean; onClose?: () => void; onChanged: () => Promise<void> }) {
  const [conversation, setConversation] = useState<AssistantConversationView | null>(null)
  const [conversations, setConversations] = useState<AssistantConversation[]>([])
  const [composer, setComposer] = useState("")
  const [attachments, setAttachments] = useState<File[]>([])
  const [dragging, setDragging] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [pending, setPending] = useState<{ plan?: AssistantPlanRecord; receipt?: AssistantPlanRecord; clarification?: string[] }>({})
  const endRef = useRef<HTMLDivElement>(null)
  const conversationKey = "agenda-assistant-conversation"
  const fileRef = useRef<HTMLInputElement>(null)

  const load = useCallback(async (preferredId?: string) => {
    let listed = (await organizerApi.listAssistantConversations()).conversations
    let id = preferredId || listed[0]?.id
    if (!id) {
      const created = await organizerApi.createAssistantConversation()
      listed = [created]
      id = created.id
    }
    setConversations(listed)
    window.localStorage.setItem(conversationKey, id)
    try { setConversation(await organizerApi.getAssistantConversation(id)) }
    catch (caught) {
      if (caught instanceof OrganizerApiError && caught.status === 404) {
        window.localStorage.removeItem(conversationKey)
        const created = await organizerApi.createAssistantConversation()
        window.localStorage.setItem(conversationKey, created.id)
        setConversations((current) => [created, ...current.filter((value) => value.id !== created.id)])
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
        const capture = await organizerApi.createCapture(text, attachments)
        await organizerApi.parseCapture(capture.id)
        await organizerApi.attachAssistantCapture(conversation.id, capture.id)
        setComposer(""); setAttachments([])
        await onChanged()
        await load(conversation.id)
        return
      }
      const response = await organizerApi.submitAssistantTurn(conversation.id, text, makeKey())
      setComposer("")
      setPending({ plan: response.plan, receipt: response.receipt, clarification: response.clarificationQuestions })
      await load(conversation.id)
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

  async function act(action: "confirm" | "cancel") {
    if (!pending.plan || busy) return
    setBusy(true); setError("")
    try {
      const response = action === "confirm" ? await organizerApi.confirmAssistantPlan(pending.plan.id) : await organizerApi.cancelAssistantPlan(pending.plan.id)
      await load(pending.plan.conversationId ?? conversation!.id)
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
      await load(conversation?.id); await onChanged()
    } catch (caught) { setError(`撤销失败：${errorMessage(caught)}`) }
    finally { setBusy(false) }
  }

  const turns = conversation?.turns ?? []
  async function startConversation() {
    if (busy) return
    setBusy(true); setError(""); setPending({})
    try {
      const created = await organizerApi.createAssistantConversation()
      setComposer(""); setAttachments([])
      await load(created.id)
    } catch (caught) { setError(`创建对话失败：${errorMessage(caught)}`) }
    finally { setBusy(false) }
  }

  const conversationControls = <div className="ml-auto flex min-w-0 items-center gap-1.5">
    <select value={conversation?.id ?? ""} onChange={(event) => { setPending({}); void load(event.target.value) }} disabled={busy} aria-label="切换对话" className="max-w-44 border border-border bg-background px-2 py-1.5 text-xs outline-none sm:max-w-64">
      {conversations.map((value) => <option key={value.id} value={value.id}>{value.title || "新对话"}</option>)}
    </select>
    <button type="button" onClick={() => void startConversation()} disabled={busy} aria-label="新对话" title="新对话" className="border border-border p-1.5 hover:bg-muted disabled:opacity-50"><Plus className="h-4 w-4" /></button>
  </div>
  return <section className={embedded ? "agenda-assistant-panel border-b border-border bg-card px-4 py-4 sm:px-6" : "agenda-assistant-panel fixed inset-x-0 bottom-0 z-40 flex max-h-[88vh] flex-col border-t border-border bg-background shadow-2xl sm:inset-auto sm:bottom-5 sm:right-5 sm:w-[min(440px,calc(100vw-2rem))] sm:border"}>
    {!embedded ? <header className="flex items-center gap-2 border-b border-border bg-card px-4 py-3"><div className="flex shrink-0 items-center gap-2"><CircleHelp className="h-4 w-4 text-primary" /><h2 className="text-sm font-semibold">Agenda AI</h2></div>{conversationControls}<button onClick={onClose} className="p-1.5 hover:bg-muted" aria-label="关闭"><X className="h-4 w-4" /></button></header> : <div className="mx-auto mb-3 flex max-w-5xl items-center gap-2"><CircleHelp className="h-4 w-4 text-primary" /><h2 className="text-sm font-semibold">Agenda AI</h2>{conversationControls}</div>}
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
      {(pending.plan || pending.receipt || pending.clarification?.length) ? <div className={`mr-8 border px-3 py-3 text-sm ${pending.plan?.state === "awaiting_confirmation" ? "border-primary/40 bg-primary/[0.04]" : "border-border bg-background"}`}><div className="flex items-start justify-between gap-3"><div><p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-primary">{pending.plan?.state === "awaiting_confirmation" ? "安排方案" : "执行结果"}</p><p className="mt-1 font-medium">{pending.plan?.plan.intentSummary ?? pending.receipt?.plan.intentSummary ?? "需要补充信息"}</p></div>{pending.plan?.state === "awaiting_confirmation" ? <span className="shrink-0 text-[11px] text-muted-foreground">等待你的确认</span> : null}</div>{pending.plan ? planPreview(pending.plan) : null}{pending.clarification?.map((question) => <p key={question} className="mt-2 text-amber-700 dark:text-amber-300">{question}</p>)}{(pending.receipt ?? pending.plan)?.actions.map((action) => <p key={action.actionId} className="mt-2 text-xs text-muted-foreground">{actionLabel(action.operation)}：{action.state}{action.error ? ` · ${action.error}` : ""}</p>)}{queryResults(pending.receipt ?? pending.plan)}{pending.receipt?.state === "completed" ? <button onClick={() => void undo()} className="mt-3 border border-border px-2.5 py-1.5 text-xs hover:bg-muted">撤销这次修改</button> : null}</div> : null}
      {pending.plan?.requiresConfirmation && pending.plan.state === "awaiting_confirmation" ? <div className="mr-8 flex gap-2"><button disabled={busy} onClick={() => void act("confirm")} className="flex flex-1 items-center justify-center gap-1.5 rounded-md bg-primary px-3 py-2.5 text-xs font-medium text-primary-foreground disabled:opacity-50"><Check className="h-3.5 w-3.5" />按这个方案安排</button><button disabled={busy} onClick={() => void act("cancel")} className="flex items-center gap-1.5 rounded-md border border-border px-3 py-2.5 text-xs disabled:opacity-50"><X className="h-3.5 w-3.5" />取消</button></div> : null}
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
