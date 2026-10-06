import { useCallback, useEffect, useMemo, useState } from "react"
import {
  Archive, Bell, Brain, CalendarDays, Check, CheckCircle2, ChevronRight,
  CircleDashed, Clock3, Edit3, FileImage, FolderKanban, History, Inbox,
  KeyRound, LayoutList, Loader2, LogOut, Menu, Plus, RefreshCw, RotateCcw,
  Search, Settings, Sparkles, Trash2, X, Zap,
} from "lucide-react"
import {
  organizerApi, OrganizerApiError, type Candidate, type Capture, type ItemEvent,
  type Memory, type OrganizerItem, type Project,
} from "@/lib/organizer-api"
import {
  enableNativeNotifications, getNativeNotificationStatus, isNativeAgenda, requestExactAlarmAccess,
  sendNativeTestNotification, syncNativeNotifications, type NativeNotificationStatus,
} from "@/lib/agenda-native"
import { AssistantPanel } from "./AssistantPanel"

type Session = { authenticated: boolean; pushEnabled: boolean; timezone: string }
type View = "today" | "calendar" | "projects" | "inbox" | "history" | "memory"

const navItems: { id: View; label: string; icon: typeof CalendarDays }[] = [
  { id: "today", label: "今天", icon: CalendarDays },
  { id: "calendar", label: "日程", icon: LayoutList },
  { id: "projects", label: "项目", icon: FolderKanban },
  { id: "inbox", label: "待整理", icon: Inbox },
  { id: "history", label: "历史", icon: History },
  { id: "memory", label: "记忆", icon: Brain },
]

function errorMessage(error: unknown) {
  if (error instanceof OrganizerApiError) return error.message
  if (error instanceof Error) return error.message
  return "发生未知错误"
}

function displayTime(value?: string, detailed = false) {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat("zh-CN", {
    month: "numeric", day: "numeric", weekday: detailed ? "short" : undefined,
    hour: "2-digit", minute: "2-digit", hour12: false,
  }).format(date)
}

function localInput(value?: string) {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  const offset = date.getTimezoneOffset() * 60_000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

function fromLocalInput(value: string) {
  if (!value) return ""
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "" : date.toISOString()
}

function itemMoment(item: OrganizerItem) { return item.startAt || item.dueAt || item.reminderAt || "" }
function isActive(item: OrganizerItem) { return !["done", "cancelled", "archived"].includes(item.status) }
function sameDay(value: string, target = new Date()) {
  const date = new Date(value)
  return date.getFullYear() === target.getFullYear() && date.getMonth() === target.getMonth() && date.getDate() === target.getDate()
}

function isOverdue(item: OrganizerItem, now = new Date()) {
  const value = item.dueAt || item.startAt
  if (!value) return false
  const moment = new Date(value)
  return !Number.isNaN(moment.getTime()) && moment < new Date(now.getFullYear(), now.getMonth(), now.getDate())
}

function recommendationScore(item: OrganizerItem, now = new Date()) {
  let score = (item.priority ?? 0) * 70
  if (item.status === "doing") score += 1000
  if (isOverdue(item, now)) score += 350
  const value = item.dueAt || item.startAt
  if (value) {
    const moment = new Date(value)
    if (sameDay(value, now)) score += 260
    const distance = moment.getTime() - now.getTime()
    if (distance > 0 && distance <= 72 * 60 * 60 * 1000) score += 80
  } else {
    score += 15
  }
  if (item.certainty === "tentative") score -= 35
  if (item.type === "note") score -= 30
  return score
}

function recommendationReason(item: OrganizerItem) {
  if (item.status === "doing") return "正在进行，继续推进"
  if (isOverdue(item)) return "已经逾期，需要优先处理"
  if (item.dueAt && sameDay(item.dueAt)) return `今天 ${displayTime(item.dueAt).split(" ").at(-1) ?? ""} 前完成`
  if (item.startAt && sameDay(item.startAt)) return `今天 ${displayTime(item.startAt).split(" ").at(-1) ?? ""} 开始`
  if ((item.priority ?? 0) >= 3) return "最高优先级"
  if ((item.priority ?? 0) >= 2) return "重要事项"
  return item.project ? `推进 ${item.project}` : "当前最合适的下一步行动"
}

function isAIFallback(items: Candidate[]) {
  return items.length === 1 && (items[0].confidence ?? 1) <= 0.25 && Boolean(items[0].ambiguities?.length)
}

function startOfWeek(value = new Date()) {
  const date = new Date(value)
  const day = date.getDay()
  const diff = day === 0 ? -6 : 1 - day
  date.setHours(0, 0, 0, 0)
  date.setDate(date.getDate() + diff)
  return date
}

function weekDays(anchor: Date) {
  const start = startOfWeek(anchor)
  return Array.from({ length: 7 }, (_, index) => {
    const date = new Date(start)
    date.setDate(start.getDate() + index)
    return date
  })
}

function isSameDate(left: Date, right: Date) {
  return left.getFullYear() === right.getFullYear() && left.getMonth() === right.getMonth() && left.getDate() === right.getDate()
}

function typeLabel(type: OrganizerItem["type"]) {
  return type === "event" ? "日程" : type === "reminder" ? "提醒" : type === "note" ? "记录" : "任务"
}

function itemAccent(item: OrganizerItem) {
  if (item.status === "doing") return "border-l-sky-500 bg-sky-50/80"
  if (item.type === "event") return "border-l-violet-500 bg-violet-50/80"
  if (item.type === "reminder") return "border-l-amber-500 bg-amber-50/80"
  return "border-l-primary bg-primary/5"
}

function formatScheduleDate(value: Date) {
  return value.toLocaleDateString("zh-CN", { month: "numeric", day: "numeric", weekday: "short" })
}

function QuickProjectSheet({ item, projects, onClose, onSaved }: { item: OrganizerItem; projects: Project[]; onClose: () => void; onSaved: () => Promise<void> }) {
  const [busy, setBusy] = useState(false)
  async function assign(project?: Project) {
    setBusy(true)
    try {
      await organizerApi.updateItem({ ...item, projectId: project?.id ?? "", project: project?.name ?? "" })
      await onSaved(); onClose()
    } finally { setBusy(false) }
  }
  return <div className="fixed inset-0 z-50 flex items-end bg-black/40 sm:items-center sm:justify-center" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <section className="w-full rounded-t-2xl border border-border bg-card p-4 shadow-2xl sm:max-w-md sm:rounded-xl" role="dialog" aria-modal="true" aria-labelledby="quick-project-title">
      <div className="mx-auto mb-4 h-1 w-10 rounded-full bg-border sm:hidden" /><div className="flex items-center justify-between"><div><p className="text-xs text-muted-foreground">归类事项</p><h2 id="quick-project-title" className="mt-1 font-semibold">{item.title}</h2></div><button onClick={onClose} className="rounded-md p-2 hover:bg-muted" aria-label="关闭"><X className="h-4 w-4" /></button></div>
      <div className="mt-4 grid gap-2">{projects.map((project) => <button key={project.id} disabled={busy} onClick={() => void assign(project)} className={`flex items-center justify-between rounded-lg border px-4 py-3 text-left text-sm hover:bg-muted ${item.projectId === project.id ? "border-primary bg-primary/5" : "border-border"}`}><span>{project.name}</span>{item.projectId === project.id ? <Check className="h-4 w-4 text-primary" /> : null}</button>)}<button disabled={busy} onClick={() => void assign()} className="rounded-lg border border-dashed border-border px-4 py-3 text-left text-sm text-muted-foreground hover:bg-muted">移出项目，放回未归类</button></div>
    </section>
  </div>
}

function QuickScheduleSheet({ item, onClose, onSaved }: { item: OrganizerItem; onClose: () => void; onSaved: () => Promise<void> }) {
  const [busy, setBusy] = useState(false)
  const [date, setDate] = useState(() => localInput(item.startAt || item.dueAt).slice(0, 10))
  const [time, setTime] = useState(() => localInput(item.startAt || item.dueAt).slice(11, 16) || "09:00")
  const base = new Date(); base.setHours(9, 0, 0, 0)
  const options = [0, 1, 2].map((offset) => { const value = new Date(base); value.setDate(base.getDate() + offset); return value })
  async function save(nextDate = date, nextTime = time) {
    if (!nextDate || !nextTime) return
    setBusy(true)
    try { await organizerApi.updateItem({ ...item, startAt: fromLocalInput(`${nextDate}T${nextTime}`) }); await onSaved(); onClose() }
    finally { setBusy(false) }
  }
  return <div className="fixed inset-0 z-50 flex items-end bg-black/40 sm:items-center sm:justify-center" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <section className="w-full rounded-t-2xl border border-border bg-card p-4 shadow-2xl sm:max-w-md sm:rounded-xl" role="dialog" aria-modal="true" aria-labelledby="quick-schedule-title">
      <div className="mx-auto mb-4 h-1 w-10 rounded-full bg-border sm:hidden" /><div className="flex items-center justify-between"><div><p className="text-xs text-muted-foreground">安排时间</p><h2 id="quick-schedule-title" className="mt-1 font-semibold">{item.title}</h2></div><button onClick={onClose} className="rounded-md p-2 hover:bg-muted" aria-label="关闭"><X className="h-4 w-4" /></button></div>
      <div className="mt-4 grid grid-cols-3 gap-2">{options.map((option) => <button key={option.toISOString()} disabled={busy} onClick={() => { const nextDate = option.toISOString().slice(0, 10); setDate(nextDate); void save(nextDate, time) }} className="rounded-lg border border-border px-3 py-3 text-sm hover:bg-muted">{offsetLabel(option, base)}</button>)}</div>
      <div className="mt-4 grid grid-cols-[1fr_8rem] gap-2"><label className="text-xs text-muted-foreground">日期<input type="date" value={date} onChange={(event) => setDate(event.target.value)} className="field mt-1" /></label><label className="text-xs text-muted-foreground">时间<input type="time" value={time} onChange={(event) => setTime(event.target.value)} className="field mt-1" /></label></div>
      <button disabled={busy || !date || !time} onClick={() => void save()} className="mt-4 flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-4 py-3 text-sm font-medium text-primary-foreground disabled:opacity-50">{busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Clock3 className="h-4 w-4" />}安排到 {date ? formatScheduleDate(new Date(`${date}T00:00:00`)) : "所选日期"} {time}</button>
    </section>
  </div>
}

function offsetLabel(value: Date, base: Date) {
  if (isSameDate(value, base)) return "今天"
  const tomorrow = new Date(base); tomorrow.setDate(base.getDate() + 1)
  if (isSameDate(value, tomorrow)) return "明天"
  return "后天"
}

function Login({ onAuthenticated }: { onAuthenticated: () => void }) {
  const [password, setPassword] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  async function submit(event: React.FormEvent) {
    event.preventDefault(); setBusy(true); setError("")
    try { await organizerApi.login(password); setPassword(""); onAuthenticated() }
    catch (caught) { setError(errorMessage(caught)) } finally { setBusy(false) }
  }
  return <main className="agenda-login flex min-h-screen items-center justify-center bg-background px-5 text-foreground">
    <form onSubmit={submit} className="agenda-login-card w-full max-w-sm border border-border bg-card p-7">
      <div className="mb-7 flex items-center gap-3"><div className="bg-primary/10 p-3 text-primary"><KeyRound className="h-5 w-5" /></div><div><p className="text-xs text-muted-foreground">Faber 的私人空间</p><h1 className="text-xl font-semibold">Agenda</h1></div></div>
      <label className="mb-2 block text-sm font-medium" htmlFor="agenda-password">登录密码</label>
      <input id="agenda-password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} className="w-full border border-border bg-background px-3 py-2.5 outline-none focus:ring-2 focus:ring-primary/30" required />
      {error ? <p className="mt-3 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p> : null}
      <button disabled={busy} className="mt-4 flex w-full items-center justify-center gap-2 bg-primary px-4 py-2.5 font-medium text-primary-foreground disabled:opacity-60">{busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <KeyRound className="h-4 w-4" />}登录</button>
    </form>
  </main>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <label className="block text-[11px] font-medium tracking-wide text-muted-foreground">{label}{children}</label> }

function CandidateEditor({ value, projects, onChange, onRemove }: { value: Candidate; projects: Project[]; onChange: (value: Candidate) => void; onRemove: () => void }) {
  const types: Array<[Candidate["type"], string]> = [["task", "任务"], ["event", "日程"], ["reminder", "提醒"], ["note", "记录"]]
  return <div className="agenda-editor-card space-y-4 p-4 sm:space-y-5 sm:p-5">
    <div className="flex items-start justify-between gap-3"><div className="min-w-0 flex-1"><p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">事项类型</p><div className="agenda-segmented grid grid-cols-4 gap-1 p-1">{types.map(([type, label]) => <button type="button" key={type} onClick={() => onChange({ ...value, type })} className={`rounded-md px-2 py-2 text-xs font-medium transition ${value.type === type ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground"}`}>{label}</button>)}</div></div><button type="button" onClick={onRemove} className="agenda-icon-button rounded-lg p-2 text-muted-foreground" title="移除事项" aria-label="移除事项"><Trash2 className="h-4 w-4" /></button></div>
    <input value={value.title} onChange={(e) => onChange({ ...value, title: e.target.value })} className="w-full border-0 border-b-2 border-border bg-transparent px-0 py-2 text-xl font-semibold outline-none placeholder:text-muted-foreground/50 focus:border-primary" placeholder="输入事项标题" />
    <textarea value={value.description ?? ""} onChange={(e) => onChange({ ...value, description: e.target.value })} className="agenda-description min-h-20 w-full resize-y px-3 py-3 text-sm outline-none placeholder:text-muted-foreground/60" placeholder="补充说明（可选）" />
    <div><p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">时间与提醒</p><div className="grid gap-3 sm:grid-cols-2">
      <Field label="开始时间"><input type="datetime-local" value={localInput(value.startAt)} onChange={(e) => onChange({ ...value, startAt: fromLocalInput(e.target.value) })} className="field rounded-lg" /></Field>
      <Field label="截止时间"><input type="datetime-local" value={localInput(value.dueAt)} onChange={(e) => onChange({ ...value, dueAt: fromLocalInput(e.target.value) })} className="field rounded-lg" /></Field>
      <Field label="提醒时间"><input type="datetime-local" value={localInput(value.reminderAt)} onChange={(e) => onChange({ ...value, reminderAt: fromLocalInput(e.target.value) })} className="field rounded-lg" /></Field>
      <Field label="预计用时（分钟）"><input type="number" min={0} value={value.durationMinutes ?? 0} onChange={(e) => onChange({ ...value, durationMinutes: Number(e.target.value) })} className="field rounded-lg" /></Field>
    </div></div>
    <div><p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">归类与优先级</p><div className="grid gap-3 sm:grid-cols-2">
      <Field label="项目"><input list="agenda-projects" value={value.project ?? ""} onChange={(e) => onChange({ ...value, project: e.target.value })} className="field rounded-lg" placeholder="未归类" /></Field>
      <Field label="确定性"><select value={value.certainty ?? "confirmed"} onChange={(e) => onChange({ ...value, certainty: e.target.value as Candidate["certainty"] })} className="field rounded-lg"><option value="confirmed">正式</option><option value="tentative">暂定</option></select></Field>
    </div><div className="mt-3 grid grid-cols-4 gap-2">{[0, 1, 2, 3].map((priority) => <button type="button" key={priority} onClick={() => onChange({ ...value, priority })} className={`rounded-lg border px-2 py-2 text-xs font-medium ${value.priority === priority ? "border-primary bg-primary/10 text-primary" : "border-border text-muted-foreground hover:bg-muted"}`}>P{priority} {priority === 0 ? "普通" : priority === 1 ? "关注" : priority === 2 ? "重要" : "最高"}</button>)}</div></div>
    {value.ambiguities?.length ? <p className="border-l-2 border-amber-500 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">{value.ambiguities.join("；")}</p> : null}
    <datalist id="agenda-projects">{projects.map((project) => <option key={project.id} value={project.name} />)}</datalist>
  </div>
}

function CaptureCard({ capture, projects, onChanged }: { capture: Capture; projects: Project[]; onChanged: () => Promise<void> }) {
  const [candidates, setCandidates] = useState<Candidate[]>(capture.aiResult?.items ?? [])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  async function parse() { setBusy(true); setError(""); try { const result = await organizerApi.parseCapture(capture.id); setCandidates(result.items); await onChanged() } catch (e) { setError(errorMessage(e)) } finally { setBusy(false) } }
  async function confirm() { setBusy(true); setError(""); try { await organizerApi.confirmCapture(capture.id, candidates); await onChanged() } catch (e) { setError(errorMessage(e)) } finally { setBusy(false) } }
  async function remove() {
    if (!window.confirm("彻底删除这条快速记录？原文、AI 整理结果和截图附件都会永久删除，无法恢复。")) return
    setBusy(true); setError("")
    try { await organizerApi.deleteCapture(capture.id); await onChanged() } catch (e) { setError(errorMessage(e)) } finally { setBusy(false) }
  }
  const canRetryImage = capture.hasAttachment && isAIFallback(candidates)
	const attachments = capture.attachments?.length ? capture.attachments : capture.hasAttachment ? [{ name: capture.attachmentName || "图片", mime: capture.attachmentMime || "" }] : []
  return <article className="agenda-capture-card border border-border bg-card">
	<div className="border-b border-border px-4 py-3"><div className="flex items-center justify-between gap-3 text-xs text-muted-foreground"><span className="flex items-center gap-1.5">{capture.hasAttachment ? <FileImage className="h-3.5 w-3.5" /> : <Inbox className="h-3.5 w-3.5" />}{new Date(capture.createdAt).toLocaleString("zh-CN")}</span><span>{capture.status === "needs_review" ? "等待处理" : capture.status}</span></div><p className="mt-2 whitespace-pre-wrap text-sm">{capture.rawText || attachments.map((attachment) => attachment.name).join("、") || "图片记录"}</p>{attachments.length ? <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1">{attachments.map((attachment, index) => <a key={`${attachment.name}-${index}`} className="text-xs text-primary hover:underline" href={index === 0 ? `/api/organizer/v1/captures/${capture.id}/attachment` : `/api/organizer/v1/captures/${capture.id}/attachments/${index}`} target="_blank" rel="noreferrer">查看图片 {index + 1}</a>)}</div> : null}</div>
    <div className="space-y-3 p-4">{candidates.map((candidate, index) => <CandidateEditor key={index} value={candidate} projects={projects} onChange={(next) => setCandidates((all) => all.map((item, i) => i === index ? next : item))} onRemove={() => setCandidates((all) => all.filter((_, i) => i !== index))} />)}
      {error || capture.error ? <p className="bg-destructive/10 px-3 py-2 text-sm text-destructive">{error || capture.error}</p> : null}
      <div className="flex flex-wrap items-center justify-between gap-2"><div>{candidates.length === 0 ? <button disabled={busy} onClick={parse} className="flex items-center gap-2 bg-secondary px-3 py-2 text-sm font-medium disabled:opacity-50">{busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Sparkles className="h-4 w-4" />}AI 整理</button> : <div className="flex flex-wrap gap-2"><button disabled={busy || candidates.some((v) => !v.title.trim())} onClick={confirm} className="flex items-center gap-2 bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50">{busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}确认并安排</button>{canRetryImage ? <button disabled={busy} onClick={parse} className="flex items-center gap-2 border border-border px-3 py-2 text-sm font-medium hover:bg-muted disabled:opacity-50"><RefreshCw className={`h-4 w-4 ${busy ? "animate-spin" : ""}`} />重新识别图片</button> : null}</div>}</div><button disabled={busy} onClick={remove} className="flex items-center gap-1.5 px-2 py-2 text-sm text-muted-foreground hover:bg-destructive/10 hover:text-destructive disabled:opacity-50"><Trash2 className="h-4 w-4" />彻底删除</button></div>
    </div>
  </article>
}

function ItemDialog({ item, projects, onClose, onSaved }: { item?: OrganizerItem; projects: Project[]; onClose: () => void; onSaved: () => Promise<void> }) {
  const empty: Candidate = { type: "task", title: "", certainty: "confirmed", priority: 0, durationMinutes: 0 }
  const [value, setValue] = useState<Candidate>(item ?? empty)
  const [status, setStatus] = useState(item?.status ?? "todo")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  async function save() {
    if (!value.title.trim()) return
    setBusy(true); setError("")
    try { if (item) await organizerApi.updateItem({ ...item, ...value, status }); else await organizerApi.createItem(value); await onSaved(); onClose() }
    catch (e) { setError(errorMessage(e)) } finally { setBusy(false) }
  }
  return <div className="agenda-dialog-backdrop fixed inset-0 z-50 flex items-end justify-center p-0 sm:items-center sm:p-6" onMouseDown={(e) => { if (e.currentTarget === e.target) onClose() }}>
    <section role="dialog" aria-modal="true" aria-labelledby="agenda-item-dialog-title" className="agenda-dialog flex max-h-[94vh] w-full max-w-2xl flex-col overflow-hidden sm:max-h-[90vh]"><header className="agenda-dialog-header flex shrink-0 items-center justify-between"><div><p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-primary">{item ? "事项详情" : "新事项"}</p><h2 id="agenda-item-dialog-title" className="mt-1 text-lg font-semibold">{item ? "编辑事项" : "创建一件新事"}</h2></div><button onClick={onClose} className="agenda-icon-button p-2.5 text-muted-foreground" aria-label="关闭"><X className="h-5 w-5" /></button></header>
      <div className="agenda-dialog-scroll min-h-0 flex-1 space-y-4 overflow-y-auto p-4 sm:p-6"><CandidateEditor value={value} projects={projects} onChange={setValue} onRemove={onClose} />{item ? <div className="agenda-status-section"><Field label="状态"><select value={status} onChange={(e) => setStatus(e.target.value as OrganizerItem["status"])} className="field rounded-lg"><option value="todo">待办</option><option value="doing">进行中</option><option value="done">已完成</option><option value="cancelled">已取消</option><option value="archived">已归档</option></select></Field></div> : null}{error ? <p className="rounded-lg border border-destructive/20 bg-destructive/10 px-3 py-3 text-sm text-destructive">{error}</p> : null}</div>
      <footer className="agenda-dialog-footer flex shrink-0 items-center justify-between gap-3 sm:justify-end"><button onClick={onClose} className="agenda-secondary-action px-4 py-2.5 text-sm text-muted-foreground">取消</button><button onClick={save} disabled={busy || !value.title.trim()} className="agenda-primary-action flex min-w-32 items-center justify-center gap-2 px-5 py-2.5 text-sm font-semibold text-primary-foreground disabled:opacity-50">{busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}保存事项</button></footer>
    </section>
  </div>
}

function WeekTimeline({ items, onEdit }: { items: OrganizerItem[]; onEdit: (item: OrganizerItem) => void }) {
  const [anchor, setAnchor] = useState(new Date())
  const [selectedDayKey, setSelectedDayKey] = useState("")
  const days = weekDays(anchor)
  const today = new Date()
  const rangeLabel = `${days[0].toLocaleDateString("zh-CN", { month: "short", day: "numeric" })} - ${days[6].toLocaleDateString("zh-CN", { month: "short", day: "numeric" })}`
  const scheduled = items.filter((item) => item.startAt || item.dueAt)
  const timedMinutes = scheduled.map((item) => {
    const value = item.startAt || item.dueAt
    return value ? new Date(value).getHours() * 60 + new Date(value).getMinutes() : 0
  }).filter((value) => Number.isFinite(value))
  const startHour = Math.max(6, Math.floor((Math.min(...timedMinutes, 6 * 60) - 30) / 60))
  const endHour = Math.min(23, Math.ceil((Math.max(...timedMinutes, 18 * 60) + 90) / 60))
  const rangeMinutes = Math.max(60, (endHour - startHour) * 60)
  const hours = Array.from({ length: endHour - startHour + 1 }, (_, index) => startHour + index)
  const selectedDay = days.find((day) => day.toISOString().slice(0, 10) === selectedDayKey) ?? days.find((day) => isSameDate(day, today)) ?? days[0]
  const selectedItems = scheduled.filter((item) => {
    const value = item.startAt || item.dueAt
    return value ? isSameDate(new Date(value), selectedDay) : false
  }).sort((left, right) => new Date(left.startAt || left.dueAt || 0).getTime() - new Date(right.startAt || right.dueAt || 0).getTime())

  function moveWeek(amount: number) {
    setAnchor((current) => {
      const next = new Date(current)
      next.setDate(next.getDate() + amount * 7)
      return next
    })
    setSelectedDayKey("")
  }

  function itemStyle(item: OrganizerItem) {
    const value = item.startAt || item.dueAt
    if (!value) return undefined
    const date = new Date(value)
    const minutes = date.getHours() * 60 + date.getMinutes()
    const top = Math.max(0, minutes - startHour * 60)
    const duration = Math.min(item.durationMinutes || 45, 180)
    return { top: `${(top / rangeMinutes) * 100}%`, height: `${(duration / rangeMinutes) * 100}%` }
  }

  return <section className="agenda-surface overflow-hidden">
    <header className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3">
      <div><p className="text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">周视图</p><h2 className="mt-1 text-base font-semibold">{rangeLabel}</h2></div>
      <div className="flex items-center gap-1"><button onClick={() => moveWeek(-1)} className="border border-border px-2.5 py-1.5 text-sm hover:bg-muted" aria-label="上一周">‹</button><button onClick={() => setAnchor(new Date())} className="border border-border px-3 py-1.5 text-sm hover:bg-muted">本周</button><button onClick={() => moveWeek(1)} className="border border-border px-2.5 py-1.5 text-sm hover:bg-muted" aria-label="下一周">›</button></div>
    </header>
    <div className="w-full overflow-x-auto">
      <div className="w-full">
        <div className="grid grid-cols-[30px_repeat(7,minmax(0,1fr))] border-b border-border bg-muted/30 text-center text-[10px] sm:grid-cols-[46px_repeat(7,minmax(0,1fr))] sm:text-xs md:min-w-[860px] md:grid-cols-[54px_repeat(7,minmax(112px,1fr))]">
          <div />
          {days.map((day) => { const key = day.toISOString().slice(0, 10); const dayCount = scheduled.filter((item) => { const value = item.startAt || item.dueAt; return value ? isSameDate(new Date(value), day) : false }).length; const selected = selectedDayKey ? selectedDayKey === key : isSameDate(day, selectedDay); return <button type="button" key={day.toISOString()} onClick={() => setSelectedDayKey(key)} className={`border-l border-border py-2 sm:px-1 sm:py-3 ${selected ? "bg-primary/15 text-primary" : isSameDate(day, today) ? "bg-primary/10 text-primary" : "text-muted-foreground"}`}><p>{day.toLocaleDateString("zh-CN", { weekday: "narrow" })}</p><p className="mt-0.5 text-xs font-semibold sm:mt-1 sm:text-base">{day.getDate()}</p><span className="mt-0.5 block text-[8px] font-normal opacity-70 sm:text-[10px]">{dayCount ? `${dayCount}项` : ""}</span></button> })}
        </div>
        <div className="relative grid grid-cols-[30px_repeat(7,minmax(0,1fr))] sm:grid-cols-[46px_repeat(7,minmax(0,1fr))] md:min-w-[860px] md:grid-cols-[54px_repeat(7,minmax(112px,1fr))]">
          <div className="relative h-[760px] sm:h-[980px] md:h-[1250px]">{hours.map((hour) => <span key={hour} className="absolute right-0.5 -translate-y-1/2 text-[8px] leading-none text-muted-foreground sm:right-1 sm:text-[9px] md:right-2 md:text-[10px]" style={{ top: `${(((hour - startHour) * 60) / rangeMinutes) * 100}%` }}><span className="md:hidden">{`${hour}`.padStart(2, "0")}</span><span className="hidden md:inline">{`${hour}`.padStart(2, "0")}:00</span></span>)}</div>
          {days.map((day) => <div key={day.toISOString()} className={`relative h-[760px] border-l border-border sm:h-[980px] md:h-[1250px] ${isSameDate(day, today) ? "bg-primary/[0.025]" : ""}`}>
            {hours.map((hour) => <div key={hour} className="absolute inset-x-0 border-t border-border/60" style={{ top: `${(((hour - startHour) * 60) / rangeMinutes) * 100}%` }} />)}
            {scheduled.filter((item) => { const value = item.startAt || item.dueAt; return value ? isSameDate(new Date(value), day) : false }).map((item) => <button key={item.id} onClick={() => onEdit(item)} aria-label={`${displayTime(item.startAt || item.dueAt, true)} ${item.title}`} className={`absolute inset-x-px z-10 min-h-7 overflow-hidden rounded border-l-2 px-0.5 py-1 text-left leading-tight shadow-sm transition sm:inset-x-0.5 sm:min-h-9 sm:border-l-4 sm:px-1 md:inset-x-1 md:min-h-12 md:rounded-md md:px-2 md:py-1.5 ${itemAccent(item)}`} style={itemStyle(item)}><p className="line-clamp-2 break-words text-[9px] font-semibold sm:text-[10px] md:line-clamp-none md:truncate md:text-xs">{item.title}</p><p className="hidden truncate text-[10px] text-muted-foreground md:mt-1 md:block">{typeLabel(item.type)} {displayTime(item.startAt || item.dueAt).split(" ").at(-1)}</p></button>)}
          </div>)}
        </div>
      </div>
    </div>
    <section className="border-t border-border bg-muted/20 px-3 py-3 sm:px-4" aria-labelledby="selected-day-title">
      <div className="flex items-center justify-between gap-3"><div><p className="text-[11px] text-muted-foreground">当天详情</p><h3 id="selected-day-title" className="mt-0.5 text-sm font-semibold">{selectedDay.toLocaleDateString("zh-CN", { month: "numeric", day: "numeric", weekday: "long" })}</h3></div><span className="text-xs text-muted-foreground">{selectedItems.length ? `${selectedItems.length} 项` : "暂无安排"}</span></div>
      {selectedItems.length ? <div className="mt-2 divide-y divide-border rounded-md border border-border bg-card">{selectedItems.map((item) => <button type="button" key={item.id} onClick={() => onEdit(item)} className="flex w-full items-start gap-3 px-3 py-2.5 text-left hover:bg-muted"><span className="w-12 shrink-0 pt-0.5 text-xs font-medium text-muted-foreground">{displayTime(item.startAt || item.dueAt).split(" ").at(-1)}</span><span className="min-w-0 flex-1"><span className="block text-sm font-medium">{item.title}</span><span className="mt-0.5 block text-xs text-muted-foreground">{typeLabel(item.type)}{item.project ? ` · ${item.project}` : ""}{item.durationMinutes ? ` · ${item.durationMinutes} 分钟` : ""}</span></span><ChevronRight className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" /></button>)}</div> : <p className="mt-2 text-xs text-muted-foreground">点击上方日期切换当天，或为这一天安排事项。</p>}
    </section>
    {!scheduled.length ? <div className="px-5 py-10 text-center text-sm text-muted-foreground">这一周还没有安排时间的事项</div> : null}
    <div className="border-t border-border px-3 py-2 text-[11px] text-muted-foreground sm:px-4 sm:py-3 sm:text-xs">点击事项查看详情；没有时间的事项会收纳在下方。</div>
  </section>
}

function ItemRow({ item, onEdit, onChanged, history = false, draggable = false, onDragStart, onProject, onSchedule }: { item: OrganizerItem; onEdit: () => void; onChanged: () => Promise<void>; history?: boolean; draggable?: boolean; onDragStart?: (item: OrganizerItem) => void; onProject?: () => void; onSchedule?: () => void }) {
  const [busy, setBusy] = useState(false)
  async function run(action: () => Promise<unknown>) { setBusy(true); try { await action(); await onChanged() } finally { setBusy(false) } }
  return <article draggable={draggable} onDragStart={() => onDragStart?.(item)} className={`agenda-item-row group grid grid-cols-[auto_minmax(0,1fr)_auto] gap-3 border-b border-border px-1 py-3 last:border-b-0 ${draggable ? "cursor-grab active:cursor-grabbing" : ""}`}>
    {!history ? <button aria-label="完成事项" disabled={busy} onClick={() => run(() => organizerApi.completeItem(item.id))} className="mt-0.5 flex h-5 w-5 items-center justify-center rounded-full border border-primary/50 text-primary hover:bg-primary hover:text-primary-foreground">{busy ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3 opacity-0 group-hover:opacity-100" />}</button> : <span className="mt-0.5 text-muted-foreground">{item.status === "done" ? <CheckCircle2 className="h-5 w-5 text-emerald-600" /> : item.status === "cancelled" ? <X className="h-5 w-5" /> : <Archive className="h-5 w-5" />}</span>}
    <button className="min-w-0 text-left" onClick={onEdit}><div className="flex flex-wrap items-center gap-2"><h3 className={`font-medium ${history ? "text-muted-foreground line-through decoration-border" : ""}`}>{item.title}</h3>{item.certainty === "tentative" ? <span className="border border-amber-500/50 px-1.5 py-0.5 text-[11px] text-amber-700">暂定</span> : null}{(item.priority ?? 0) > 0 ? <span className="text-xs font-medium text-orange-600">P{item.priority}</span> : null}</div>{item.description ? <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">{item.description}</p> : null}<div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">{itemMoment(item) ? <span className="flex items-center gap-1"><Clock3 className="h-3 w-3" />{displayTime(itemMoment(item), true)}</span> : <span>未安排时间</span>}{item.project ? <span className="font-medium text-foreground/70">{item.project}</span> : null}{item.durationMinutes ? <span>{item.durationMinutes} 分钟</span> : null}</div></button>
     <div className="flex items-start gap-1">{history ? <button disabled={busy} onClick={() => run(() => organizerApi.reopenItem(item.id))} className="p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground" title="重新打开"><RotateCcw className="h-4 w-4" /></button> : <><button onClick={onEdit} className="p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground" title="编辑"><Edit3 className="h-4 w-4" /></button><button onClick={onProject} className="p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground sm:hidden" title="选择项目" aria-label="选择项目"><FolderKanban className="h-4 w-4" /></button><button onClick={onSchedule} className="p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground sm:hidden" title="安排时间" aria-label="安排时间"><Clock3 className="h-4 w-4" /></button><button disabled={busy} onClick={() => run(() => organizerApi.cancelItem(item.id))} className="p-1.5 text-muted-foreground hover:bg-muted hover:text-destructive" title="取消"><X className="h-4 w-4" /></button></>}</div>
  </article>
}

function ItemList({ title, items, onEdit, onChanged, history, onProject, onSchedule }: { title?: string; items: OrganizerItem[]; onEdit: (item: OrganizerItem) => void; onChanged: () => Promise<void>; history?: boolean; onProject?: (item: OrganizerItem) => void; onSchedule?: (item: OrganizerItem) => void }) {
  if (!items.length) return null
  return <section>{title ? <div className="mb-2 flex items-center gap-2"><h2 className="text-sm font-semibold">{title}</h2><span className="text-xs text-muted-foreground">{items.length}</span></div> : null}<div className="agenda-list border-y border-border bg-card px-3">{items.map((item) => <ItemRow key={item.id} item={item} onEdit={() => onEdit(item)} onChanged={onChanged} history={history} onProject={onProject ? () => onProject(item) : undefined} onSchedule={onSchedule ? () => onSchedule(item) : undefined} />)}</div></section>
}

function FocusItem({ item, onEdit, onChanged }: { item: OrganizerItem; onEdit: () => void; onChanged: () => Promise<void> }) {
  const [busy, setBusy] = useState(false)
  async function run(action: () => Promise<unknown>) {
    setBusy(true)
    try { await action(); await onChanged() } finally { setBusy(false) }
  }
  return <section aria-labelledby="agenda-focus-title">
    <div className="mb-2 flex items-center gap-2 text-primary"><Zap className="h-4 w-4" /><h2 id="agenda-focus-title" className="text-sm font-semibold">现在最值得做</h2></div>
    <article className="agenda-focus-card border-l-4 border-primary bg-card px-4 py-4 sm:px-5">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between"><button onClick={onEdit} className="min-w-0 text-left"><p className="text-xs font-medium text-primary">{recommendationReason(item)}</p><h3 className="mt-1 text-lg font-semibold">{item.title}</h3>{item.description ? <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">{item.description}</p> : null}<div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">{itemMoment(item) ? <span className="flex items-center gap-1"><Clock3 className="h-3.5 w-3.5" />{displayTime(itemMoment(item), true)}</span> : <span>未安排时间</span>}{item.project ? <span>{item.project}</span> : null}{(item.priority ?? 0) > 0 ? <span className="font-semibold text-orange-600">P{item.priority}</span> : null}</div></button>
        <div className="flex shrink-0 gap-2"><button disabled={busy} onClick={() => item.status === "doing" ? onEdit() : run(() => organizerApi.updateItem({ ...item, status: "doing" }))} className="flex items-center gap-2 border border-primary px-3 py-2 text-sm font-medium text-primary hover:bg-primary/5 disabled:opacity-50"><Zap className="h-4 w-4" />{item.status === "doing" ? "继续" : "开始"}</button><button aria-label="完成事项" disabled={busy} onClick={() => run(() => organizerApi.completeItem(item.id))} className="flex items-center gap-2 bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50">{busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}完成</button></div>
      </div>
    </article>
  </section>
}

function ProjectEditor({ onClose, onSaved }: { onClose: () => void; onSaved: () => Promise<void> }) {
  const [name, setName] = useState(""), [goal, setGoal] = useState(""), [busy, setBusy] = useState(false)
  async function save() { setBusy(true); try { await organizerApi.createProject({ name, goal, status: "active", priority: 0 }); await onSaved(); onClose() } finally { setBusy(false) } }
  return <div className="flex gap-2 border border-border bg-card p-3"><input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="项目名称" className="min-w-0 flex-1 border border-border bg-background px-3 py-2 text-sm" /><input value={goal} onChange={(e) => setGoal(e.target.value)} placeholder="项目目标（可选）" className="hidden min-w-0 flex-[2] border border-border bg-background px-3 py-2 text-sm sm:block" /><button onClick={save} disabled={busy || !name.trim()} className="bg-primary px-3 text-sm text-primary-foreground">创建</button><button onClick={onClose} className="p-2"><X className="h-4 w-4" /></button></div>
}

function NativeNotificationPanel({ items, onMessage }: { items: OrganizerItem[]; onMessage: (value: string) => void }) {
  const [status, setStatus] = useState<NativeNotificationStatus>()
  const [busy, setBusy] = useState(false)
  const refreshStatus = useCallback(async () => setStatus(await getNativeNotificationStatus()), [])
  useEffect(() => { void refreshStatus() }, [refreshStatus])
  async function run(action: () => Promise<unknown>, message?: string) {
    setBusy(true)
    try { await action(); await refreshStatus(); if (message) onMessage(message) }
    finally { setBusy(false) }
  }
  if (!isNativeAgenda()) return null
  return <section className="space-y-3"><h3 className="font-medium">本机通知</h3>
    <div className="grid grid-cols-3 border border-border text-center text-xs"><div className="p-3"><p className="text-muted-foreground">通知权限</p><p className="mt-1 font-medium">{status?.permission === "granted" ? "已允许" : "未允许"}</p></div><div className="border-l border-border p-3"><p className="text-muted-foreground">精确提醒</p><p className="mt-1 font-medium">{status?.exactAlarm === "granted" ? "已允许" : "需设置"}</p></div><div className="border-l border-border p-3"><p className="text-muted-foreground">已安排</p><p className="mt-1 font-medium">{status?.scheduled ?? "…"} 条</p></div></div>
    <div className="flex flex-wrap gap-2"><button disabled={busy} onClick={() => void run(() => enableNativeNotifications(items), "本机提醒已同步。")} className="bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50">开启并同步</button><button disabled={busy} onClick={() => void run(requestExactAlarmAccess)} className="border border-border px-3 py-2 text-sm disabled:opacity-50">精确提醒设置</button><button disabled={busy} onClick={() => void run(sendNativeTestNotification, "测试通知将在 1 分钟后出现。")} className="border border-border px-3 py-2 text-sm disabled:opacity-50">1 分钟测试</button></div>
    <p className="text-xs leading-5 text-muted-foreground">请同时在 vivo 设置中允许通知、自启动和后台高耗电。系统“强行停止”后，需要重新打开一次应用。</p>
  </section>
}

function SettingsPanel({ items, onClose, onMessage }: { items: OrganizerItem[]; onClose: () => void; onMessage: (value: string) => void }) {
  const [deviceToken, setDeviceToken] = useState(""), [currentPassword, setCurrentPassword] = useState(""), [newPassword, setNewPassword] = useState(""), [busy, setBusy] = useState(false)
  async function token() { const value = await organizerApi.createDeviceToken("Obsidian"); setDeviceToken(value.token) }
  async function password(event: React.FormEvent) { event.preventDefault(); setBusy(true); try { await organizerApi.changePassword(currentPassword, newPassword); setCurrentPassword(""); setNewPassword(""); onMessage("密码已修改。") } finally { setBusy(false) } }
  return <div className="fixed inset-0 z-50 flex justify-end bg-black/40" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose() }}><aside className="h-full w-full max-w-md overflow-y-auto border-l border-border bg-card p-6 shadow-2xl"><header className="mb-8 flex items-center justify-between"><h2 className="text-lg font-semibold">设置</h2><button onClick={onClose} className="p-2 hover:bg-muted"><X className="h-4 w-4" /></button></header><NativeNotificationPanel items={items} onMessage={onMessage} /><section className={`${isNativeAgenda() ? "mt-8 border-t border-border pt-6" : ""} space-y-3`}><h3 className="font-medium">通知与设备</h3><button onClick={token} className="border border-border px-3 py-2 text-sm">生成 Obsidian 令牌</button>{deviceToken ? <div className="break-all bg-muted p-3 font-mono text-xs select-all">{deviceToken}</div> : null}{!isNativeAgenda() ? <a className="block text-sm text-primary hover:underline" href="/api/organizer/v1/export">导出完整数据</a> : null}</section><form onSubmit={password} className="mt-8 space-y-3 border-t border-border pt-6"><h3 className="font-medium">修改密码</h3><input type="password" value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} placeholder="当前密码" className="field" required /><input type="password" minLength={12} value={newPassword} onChange={(e) => setNewPassword(e.target.value)} placeholder="新密码（至少 12 位）" className="field" required /><button disabled={busy} className="bg-secondary px-3 py-2 text-sm font-medium">修改密码</button></form></aside></div>
}

export function AgendaApp() {
  const [session, setSession] = useState<Session | null>(null), [authChecked, setAuthChecked] = useState(false)
  const [items, setItems] = useState<OrganizerItem[]>([]), [captures, setCaptures] = useState<Capture[]>([]), [projects, setProjects] = useState<Project[]>([]), [memories, setMemories] = useState<Memory[]>([]), [events, setEvents] = useState<ItemEvent[]>([])
  const [view, setView] = useState<View>("today"), [search, setSearch] = useState(""), [editing, setEditing] = useState<OrganizerItem | "new" | null>(null), [settingsOpen, setSettingsOpen] = useState(false), [newProject, setNewProject] = useState(false), [draggingItem, setDraggingItem] = useState<OrganizerItem | null>(null), [dropProjectId, setDropProjectId] = useState<string | null>(null), [quickAction, setQuickAction] = useState<{ kind: "project" | "schedule"; item: OrganizerItem } | null>(null)
  const [error, setError] = useState(""), [message, setMessage] = useState("")

  const refresh = useCallback(async () => {
    const [itemResponse, captureResponse, projectResponse, memoryResponse, eventResponse] = await Promise.all([organizerApi.listItems(), organizerApi.listCaptures(), organizerApi.listProjects(), organizerApi.listMemories(), organizerApi.listEvents()])
    setItems(itemResponse.items); setCaptures(captureResponse.captures); setProjects(projectResponse.projects); setMemories(memoryResponse.memories); setEvents(eventResponse.events)
    await syncNativeNotifications(itemResponse.items)
  }, [])
  const checkSession = useCallback(async () => { try { setSession(await organizerApi.session()); await refresh() } catch (e) { if (e instanceof OrganizerApiError && e.status === 401) setSession(null); else setError(errorMessage(e)) } finally { setAuthChecked(true) } }, [refresh])
  useEffect(() => { void checkSession() }, [checkSession])
  useEffect(() => { if (!isNativeAgenda() && "serviceWorker" in navigator) void navigator.serviceWorker.register("/agenda/sw.js", { scope: "/agenda/" }) }, [])
  useEffect(() => {
    const handleNativeRefresh = () => void refresh()
    window.addEventListener("agenda-native-refresh", handleNativeRefresh)
    return () => window.removeEventListener("agenda-native-refresh", handleNativeRefresh)
  }, [refresh])

  const filtered = useMemo(() => { const q = search.trim().toLocaleLowerCase(); return q ? items.filter((item) => [item.title, item.description, item.project, item.location, ...(item.tags ?? [])].some((value) => value?.toLocaleLowerCase().includes(q))) : items }, [items, search])
  const active = filtered.filter(isActive), history = filtered.filter((item) => !isActive(item))
  const pending = captures.filter((capture) => capture.status !== "confirmed")
  const todayItems = active.filter((item) => itemMoment(item) && sameDay(itemMoment(item)))
  const overdue = active.filter((item) => { const moment = itemMoment(item); return moment && new Date(moment) < new Date(new Date().setHours(0, 0, 0, 0)) })
  const unscheduled = active.filter((item) => !itemMoment(item))
  const focusItem = [...active].sort((a, b) => recommendationScore(b) - recommendationScore(a) || (a.dueAt || a.startAt || "9999").localeCompare(b.dueAt || b.startAt || "9999"))[0]
  const importantItems = active.filter((item) => item.id !== focusItem?.id && ((item.priority ?? 0) >= 2 || isOverdue(item))).sort((a, b) => recommendationScore(b) - recommendationScore(a)).slice(0, 6)
  const highlightedIDs = new Set([focusItem?.id, ...importantItems.map((item) => item.id)].filter(Boolean))

  async function enablePush() {
    try { if (isNativeAgenda()) { const status = await enableNativeNotifications(items); setMessage(`本机提醒已开启，已安排 ${status.scheduled} 条。`); return } if (!("serviceWorker" in navigator) || !("PushManager" in window)) throw new Error("当前浏览器不支持通知"); const permission = await Notification.requestPermission(); if (permission !== "granted") throw new Error("通知权限没有开启"); const registration = await navigator.serviceWorker.ready; const vapid = await organizerApi.vapidKey(); if (!vapid.enabled) throw new Error("服务器尚未配置推送"); let subscription = await registration.pushManager.getSubscription(); subscription ??= await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: urlBase64ToUint8Array(vapid.publicKey) }); await organizerApi.subscribe(subscription.toJSON()); setMessage("这台设备已经开启提醒。") } catch (e) { setError(errorMessage(e)) }
  }
  async function updateMemory(memory: Memory, status: Memory["status"]) { try { await organizerApi.updateMemory({ ...memory, status }); await refresh() } catch (e) { setError(errorMessage(e)) } }
  async function createMemory() { const content = window.prompt("写下希望 Agenda 记住的规则"); if (!content?.trim()) return; await organizerApi.createMemory({ content: content.trim(), kind: "preference", status: "active", scope: "global" }); await refresh() }
  async function assignToProject(project: Project) {
    if (!draggingItem || draggingItem.projectId === project.id) return
    try {
      await organizerApi.updateItem({ ...draggingItem, projectId: project.id, project: project.name })
      setMessage(`已将“${draggingItem.title}”归入 ${project.name}`)
      setDraggingItem(null); setDropProjectId(null)
      await refresh()
    } catch (e) { setError(errorMessage(e)) }
  }

  if (!authChecked) return <div className="flex min-h-screen items-center justify-center"><Loader2 className="h-7 w-7 animate-spin text-primary" /></div>
  if (!session) return <Login onAuthenticated={() => void checkSession()} />
  const title = navItems.find((item) => item.id === view)?.label ?? "Agenda"

  return <main className="agenda-root min-h-screen bg-background text-foreground">
    <div className="min-h-screen md:grid md:grid-cols-[216px_minmax(0,1fr)]">
      <aside className="agenda-sidebar hidden border-r border-border md:flex md:flex-col"><div className="border-b border-border px-5 py-5"><p className="text-xs text-muted-foreground">个人工作台</p><h1 className="mt-1 text-xl font-semibold">Agenda</h1></div><nav className="flex-1 space-y-1 p-3">{navItems.map(({ id, label, icon: Icon }) => <button key={id} onClick={() => setView(id)} className={`agenda-nav-item flex w-full items-center gap-3 px-3 py-2 text-sm ${view === id ? "is-active" : "text-muted-foreground"}`}><Icon className="h-4 w-4" />{label}{id === "inbox" && pending.length ? <span className="ml-auto text-xs">{pending.length}</span> : null}{id === "memory" && memories.some((m) => m.status === "proposed") ? <CircleDashed className="ml-auto h-3.5 w-3.5" /> : null}</button>)}</nav><div className="border-t border-border p-3"><button onClick={() => setSettingsOpen(true)} className="agenda-nav-item flex w-full items-center gap-3 px-3 py-2 text-sm text-muted-foreground"><Settings className="h-4 w-4" />设置</button></div></aside>
       <div className="agenda-content min-w-0 pb-20 md:pb-0"><header className="agenda-topbar flex h-16 items-center justify-between border-b border-border px-4 sm:px-6"><div className="flex items-center gap-3"><Menu className="h-5 w-5 md:hidden" /><div><h1 className="font-semibold">{title}</h1><p className="hidden text-xs text-muted-foreground sm:block">{view === "today" ? new Intl.DateTimeFormat("zh-CN", { month: "long", day: "numeric", weekday: "long" }).format(new Date()) : "管理你的日程、项目与历史"}</p></div></div><div className="flex items-center gap-1"><button onClick={() => setEditing("new")} className="agenda-primary-action flex items-center gap-2 rounded-md px-3 py-2 text-sm text-primary-foreground"><Plus className="h-4 w-4" /><span className="hidden sm:inline">新增</span></button><button onClick={() => void refresh()} className="agenda-icon-button rounded-md p-2.5" aria-label="刷新"><RefreshCw className="h-4 w-4" /></button><button onClick={enablePush} className="agenda-icon-button rounded-md p-2.5" aria-label="通知"><Bell className="h-4 w-4" /></button><button onClick={() => setSettingsOpen(true)} className="agenda-icon-button rounded-md p-2.5 md:hidden" aria-label="设置"><Settings className="h-4 w-4" /></button><button onClick={async () => { await organizerApi.logout(); setSession(null) }} className="agenda-icon-button rounded-md p-2.5" aria-label="退出"><LogOut className="h-4 w-4" /></button></div></header>
        {(view === "today" || view === "calendar" || view === "projects") ? <AssistantPanel embedded onChanged={refresh} /> : null}
        <div className="agenda-content-inner mx-auto max-w-6xl px-4 py-6 sm:px-6">
          {(view === "calendar" || view === "projects" || view === "history") ? <div className="agenda-search mb-5 flex items-center gap-2 px-3"><Search className="h-4 w-4 text-muted-foreground" /><input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="搜索标题、项目、标签或地点" className="h-10 min-w-0 flex-1 bg-transparent text-sm outline-none" />{search ? <button onClick={() => setSearch("")} className="agenda-icon-button p-1"><X className="h-4 w-4" /></button> : null}</div> : null}
          {error ? <div className="mb-4 bg-destructive/10 px-4 py-3 text-sm text-destructive">{error}<button onClick={() => setError("")} className="float-right"><X className="h-4 w-4" /></button></div> : null}{message ? <div className="mb-4 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-700">{message}</div> : null}

           {view === "today" ? <div className="space-y-7"><section className="agenda-hero-card rounded-lg border border-border bg-card p-5 sm:p-6"><div className="flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between"><div><p className="text-xs font-semibold uppercase tracking-[0.16em] text-primary">今日节奏</p><h2 className="mt-2 text-2xl font-semibold tracking-tight">把注意力放在一件事上</h2><p className="mt-2 text-sm text-muted-foreground">{todayItems.length ? `今天有 ${todayItems.length} 件已安排事项，先从最重要的一步开始。` : "今天还没有安排事项。"}</p></div><div className="min-w-[180px]"><div className="flex items-center justify-between text-xs text-muted-foreground"><span>当前进度</span><span>{history.filter((item) => item.status === "done" && item.completedAt && sameDay(item.completedAt)).length} / {todayItems.length + history.filter((item) => item.status === "done" && item.completedAt && sameDay(item.completedAt)).length || 0}</span></div><div className="mt-2 h-2 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-primary transition-all" style={{ width: `${Math.min(100, Math.round((history.filter((item) => item.status === "done" && item.completedAt && sameDay(item.completedAt)).length / Math.max(1, todayItems.length + history.filter((item) => item.status === "done" && item.completedAt && sameDay(item.completedAt)).length)) * 100))}%` }} /></div></div></div></section>{focusItem ? <FocusItem item={focusItem} onEdit={() => setEditing(focusItem)} onChanged={refresh} /> : null}{importantItems.length ? <ItemList title="优先处理" items={importantItems} onEdit={setEditing} onChanged={refresh} onProject={(item) => setQuickAction({ kind: "project", item })} onSchedule={(item) => setQuickAction({ kind: "schedule", item })} /> : null}<section className="agenda-stats grid gap-px overflow-hidden rounded-lg border border-border bg-border sm:grid-cols-4">{[["今天", todayItems.length], ["逾期", overdue.length], ["待整理", pending.length], ["活跃项目", projects.filter((p) => p.status === "active").length]].map(([label, count]) => <div key={label} className="bg-card px-4 py-4"><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 text-2xl font-semibold">{count}</p></div>)}</section><ItemList title="已逾期" items={overdue.filter((item) => !highlightedIDs.has(item.id))} onEdit={setEditing} onChanged={refresh} onProject={(item) => setQuickAction({ kind: "project", item })} onSchedule={(item) => setQuickAction({ kind: "schedule", item })} /><ItemList title="今天" items={todayItems.filter((item) => !overdue.includes(item) && !highlightedIDs.has(item.id))} onEdit={setEditing} onChanged={refresh} onProject={(item) => setQuickAction({ kind: "project", item })} onSchedule={(item) => setQuickAction({ kind: "schedule", item })} />{unscheduled.length ? <details className="agenda-surface"><summary className="flex cursor-pointer list-none items-center justify-between px-4 py-3"><span><span className="text-sm font-semibold">未安排</span><span className="ml-2 text-xs text-muted-foreground">{unscheduled.length} 件</span></span><span className="text-xs text-muted-foreground">需要时再展开</span></summary><div className="border-t border-border px-3"><ItemList items={unscheduled.filter((item) => !highlightedIDs.has(item.id)).slice(0, 12)} onEdit={setEditing} onChanged={refresh} onProject={(item) => setQuickAction({ kind: "project", item })} onSchedule={(item) => setQuickAction({ kind: "schedule", item })} /></div></details> : null}{!active.length ? <Empty text="今天没有待处理事项。" /> : null}{history.filter((item) => item.status === "done" && item.completedAt && sameDay(item.completedAt)).length ? <details><summary className="cursor-pointer text-sm font-medium text-muted-foreground">今天已完成 {history.filter((item) => item.status === "done" && item.completedAt && sameDay(item.completedAt)).length} 项</summary><div className="mt-2"><ItemList items={history.filter((item) => item.status === "done" && item.completedAt && sameDay(item.completedAt))} onEdit={setEditing} onChanged={refresh} history /></div></details> : null}</div> : null}

           {view === "calendar" ? <div className="space-y-5"><WeekTimeline items={active.filter((item) => itemMoment(item))} onEdit={setEditing} />{unscheduled.length ? <details className="rounded-lg border border-border bg-card shadow-sm"><summary className="flex cursor-pointer list-none items-center justify-between px-4 py-3"><span className="text-sm font-semibold">未安排 <span className="ml-1 text-xs font-normal text-muted-foreground">{unscheduled.length}</span></span><span className="text-xs text-muted-foreground">已收起</span></summary><div className="border-t border-border px-4 py-2"><ItemList items={unscheduled} onEdit={setEditing} onChanged={refresh} onProject={(item) => setQuickAction({ kind: "project", item })} onSchedule={(item) => setQuickAction({ kind: "schedule", item })} /></div></details> : null}{!active.length ? <Empty text="没有进行中的日程或任务。" /> : null}</div> : null}

           {view === "projects" ? <div className="space-y-4"><div className="flex items-center justify-between"><div><p className="text-sm font-medium">把事项放回它所属的上下文</p><p className="mt-1 text-xs text-muted-foreground">手机端点击项目图标即可归类，桌面端也可以拖动</p></div><button onClick={() => setNewProject(true)} className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm hover:bg-muted"><Plus className="h-4 w-4" />新项目</button></div>{newProject ? <ProjectEditor onClose={() => setNewProject(false)} onSaved={refresh} /> : null}<div className="grid gap-4 lg:grid-cols-2">{projects.map((project) => { const projectItems = filtered.filter((item) => item.projectId === project.id || item.project === project.name); const isDropTarget = dropProjectId === project.id; return <section key={project.id} onDragOver={(event) => { event.preventDefault(); setDropProjectId(project.id) }} onDragLeave={() => setDropProjectId(null)} onDrop={(event) => { event.preventDefault(); void assignToProject(project) }} className={`rounded-lg border bg-card shadow-sm transition ${isDropTarget ? "border-primary ring-2 ring-primary/20" : "border-border"}`}><header className="border-b border-border px-4 py-3"><div className="flex items-center justify-between gap-3"><div><h2 className="font-semibold">{project.name}</h2><p className="mt-0.5 text-xs text-muted-foreground">{project.status === "active" ? "进行中" : project.status} · {project.openCount} 项待处理 · {project.doneCount} 项完成</p></div><ChevronRight className="h-4 w-4 text-muted-foreground" /></div>{project.goal ? <p className="mt-2 text-sm text-muted-foreground">{project.goal}</p> : null}</header><div className="px-3">{projectItems.filter(isActive).slice(0, 5).map((item) => <ItemRow key={item.id} item={item} draggable onDragStart={setDraggingItem} onProject={() => setQuickAction({ kind: "project", item })} onSchedule={() => setQuickAction({ kind: "schedule", item })} onEdit={() => setEditing(item)} onChanged={refresh} />)}{!projectItems.filter(isActive).length ? <p className="py-5 text-center text-sm text-muted-foreground">拖动事项到这里</p> : null}</div></section>})}</div>{filtered.filter((item) => isActive(item) && !item.projectId && !item.project).length ? <section className="rounded-lg border border-dashed border-primary/40 bg-primary/[0.03] shadow-sm"><header className="flex items-center justify-between border-b border-primary/20 px-4 py-3"><div><h2 className="text-sm font-semibold">未归类事项</h2><p className="mt-1 text-xs text-muted-foreground">从这里拖到上面的项目，也可以点击项目图标</p></div><span className="text-xs text-muted-foreground">{filtered.filter((item) => isActive(item) && !item.projectId && !item.project).length} 件</span></header><div className="px-3">{filtered.filter((item) => isActive(item) && !item.projectId && !item.project).map((item) => <ItemRow key={item.id} item={item} draggable onDragStart={setDraggingItem} onProject={() => setQuickAction({ kind: "project", item })} onSchedule={() => setQuickAction({ kind: "schedule", item })} onEdit={() => setEditing(item)} onChanged={refresh} />)}</div></section> : null}{!projects.length ? <Empty text="还没有项目。创建项目后，Agenda 会按项目汇总工作。" /> : null}</div> : null}

          {view === "inbox" ? <div className="space-y-4"><div><h2 className="font-semibold">等待你整理</h2><p className="mt-1 text-sm text-muted-foreground">这里保存刚记录的内容和 AI 建议。确认后会进入日程或项目，但原始内容仍会保留。</p></div>{pending.map((capture) => <CaptureCard key={`${capture.id}-${capture.updatedAt}`} capture={capture} projects={projects} onChanged={refresh} />)}{!pending.length ? <Empty text="没有等待整理的内容。" /> : null}</div> : null}

          {view === "history" ? <div className="space-y-7"><section className="flex gap-5 border-b border-border pb-4 text-sm text-muted-foreground"><span>完成 {history.filter((i) => i.status === "done").length}</span><span>取消 {history.filter((i) => i.status === "cancelled").length}</span><span>归档 {history.filter((i) => i.status === "archived").length}</span><span>活动记录 {events.length}</span></section><ItemList title="已完成" items={history.filter((i) => i.status === "done").sort((a, b) => (b.completedAt ?? b.updatedAt).localeCompare(a.completedAt ?? a.updatedAt))} onEdit={setEditing} onChanged={refresh} history /><ItemList title="已取消" items={history.filter((i) => i.status === "cancelled")} onEdit={setEditing} onChanged={refresh} history /><ItemList title="已归档" items={history.filter((i) => i.status === "archived")} onEdit={setEditing} onChanged={refresh} history />{!history.length ? <Empty text="完成、取消和归档的内容会保留在这里。" /> : null}</div> : null}

          {view === "memory" ? <div className="space-y-6"><div className="flex items-start justify-between gap-4"><div><h2 className="font-semibold">Agenda 记住的规则</h2><p className="mt-1 max-w-2xl text-sm text-muted-foreground">只有已批准规则会参与后续解析。重复纠正形成的规律会先等待你确认。</p></div><button onClick={() => void createMemory()} className="flex shrink-0 items-center gap-2 border border-border px-3 py-2 text-sm hover:bg-muted"><Plus className="h-4 w-4" />添加规则</button></div>{memories.filter((m) => m.status === "proposed").length ? <section><h3 className="mb-2 text-sm font-semibold">待你决定</h3><div className="divide-y divide-border border border-border bg-card">{memories.filter((m) => m.status === "proposed").map((memory) => <div key={memory.id} className="p-4"><p className="text-sm">{memory.content}</p><p className="mt-1 text-xs text-muted-foreground">来自 {memory.evidenceCount} 次相似纠正</p><div className="mt-3 flex gap-2"><button onClick={() => void updateMemory(memory, "active")} className="bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground">记住</button><button onClick={() => void updateMemory(memory, "dismissed")} className="border border-border px-3 py-1.5 text-xs">忽略</button></div></div>)}</div></section> : null}<section><h3 className="mb-2 text-sm font-semibold">正在使用</h3><div className="divide-y divide-border border border-border bg-card">{memories.filter((m) => m.status === "active").map((memory) => <div key={memory.id} className="flex items-start justify-between gap-4 p-4"><div><p className="text-sm">{memory.content}</p><p className="mt-1 text-xs text-muted-foreground">{memory.kind === "procedure" ? "助手规则" : "个人偏好"} · {memory.scope}</p></div><button onClick={() => void updateMemory(memory, "forgotten")} className="p-2 text-muted-foreground hover:bg-muted hover:text-destructive" title="忘记"><Trash2 className="h-4 w-4" /></button></div>)}{!memories.some((m) => m.status === "active") ? <p className="p-6 text-center text-sm text-muted-foreground">还没有已批准的个人规则。</p> : null}</div></section></div> : null}
        </div>
      </div>
    </div>
    <nav className="agenda-bottom-nav fixed inset-x-0 bottom-0 z-30 grid grid-cols-6 border-t border-border md:hidden">{navItems.map(({ id, label, icon: Icon }) => <button key={id} onClick={() => setView(id)} className={`agenda-bottom-nav-item flex h-16 flex-col items-center justify-center gap-1 text-[10px] ${view === id ? "is-active" : "text-muted-foreground"}`}><Icon className="h-4 w-4" />{label}</button>)}</nav>
    {editing ? <ItemDialog item={editing === "new" ? undefined : editing} projects={projects} onClose={() => setEditing(null)} onSaved={refresh} /> : null}
    {settingsOpen ? <SettingsPanel items={items} onClose={() => setSettingsOpen(false)} onMessage={setMessage} /> : null}
    {quickAction?.kind === "project" ? <QuickProjectSheet item={quickAction.item} projects={projects} onClose={() => setQuickAction(null)} onSaved={refresh} /> : null}
    {quickAction?.kind === "schedule" ? <QuickScheduleSheet item={quickAction.item} onClose={() => setQuickAction(null)} onSaved={refresh} /> : null}
  </main>
}

function Empty({ text }: { text: string }) { return <div className="border border-dashed border-border px-5 py-10 text-center text-sm text-muted-foreground">{text}</div> }

function urlBase64ToUint8Array(value: string) {
  const padding = "=".repeat((4 - value.length % 4) % 4), base64 = (value + padding).replace(/-/g, "+").replace(/_/g, "/"), raw = window.atob(base64)
  return Uint8Array.from([...raw].map((char) => char.charCodeAt(0)))
}
