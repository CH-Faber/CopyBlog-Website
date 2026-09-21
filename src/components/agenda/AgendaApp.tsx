import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import {
  Archive, Bell, Brain, CalendarDays, Check, CheckCircle2, ChevronRight,
  CircleDashed, Clock3, Edit3, FileImage, FolderKanban, History, Inbox,
  KeyRound, LayoutList, Loader2, LogOut, Menu, Plus, RefreshCw, RotateCcw,
  Search, Send, Settings, Sparkles, Trash2, X,
} from "lucide-react"
import {
  organizerApi, OrganizerApiError, type Candidate, type Capture, type ItemEvent,
  type Memory, type OrganizerItem, type Project,
} from "@/lib/organizer-api"

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
function dayKey(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "未安排" : new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "long", day: "numeric", weekday: "short" }).format(date)
}
function sameDay(value: string, target = new Date()) {
  const date = new Date(value)
  return date.getFullYear() === target.getFullYear() && date.getMonth() === target.getMonth() && date.getDate() === target.getDate()
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
  return <main className="flex min-h-screen items-center justify-center bg-background px-5 text-foreground">
    <form onSubmit={submit} className="w-full max-w-sm border border-border bg-card p-7 shadow-lg">
      <div className="mb-7 flex items-center gap-3"><div className="bg-primary/10 p-3 text-primary"><KeyRound className="h-5 w-5" /></div><div><p className="text-xs text-muted-foreground">Faber 的私人空间</p><h1 className="text-xl font-semibold">Agenda</h1></div></div>
      <label className="mb-2 block text-sm font-medium" htmlFor="agenda-password">登录密码</label>
      <input id="agenda-password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} className="w-full border border-border bg-background px-3 py-2.5 outline-none focus:ring-2 focus:ring-primary/30" required />
      {error ? <p className="mt-3 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p> : null}
      <button disabled={busy} className="mt-4 flex w-full items-center justify-center gap-2 bg-primary px-4 py-2.5 font-medium text-primary-foreground disabled:opacity-60">{busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <KeyRound className="h-4 w-4" />}登录</button>
    </form>
  </main>
}

function QuickCapture({ onCreated }: { onCreated: () => Promise<void> }) {
  const [text, setText] = useState("")
  const [attachment, setAttachment] = useState<File>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const fileRef = useRef<HTMLInputElement>(null)
  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!text.trim() && !attachment) return
    setBusy(true); setError("")
    try {
      const capture = await organizerApi.createCapture(text.trim(), attachment)
      setText(""); setAttachment(undefined); if (fileRef.current) fileRef.current.value = ""
      await organizerApi.parseCapture(capture.id); await onCreated()
    } catch (caught) { setError(errorMessage(caught)); await onCreated().catch(() => undefined) } finally { setBusy(false) }
  }
  return <section className="border-b border-border bg-card px-4 py-4 sm:px-6">
    <form onSubmit={submit} className="mx-auto flex max-w-5xl items-end gap-2">
      <div className="min-w-0 flex-1"><label htmlFor="agenda-quick-capture" className="mb-1 block text-xs font-medium text-muted-foreground">快速记录</label><textarea id="agenda-quick-capture" rows={2} value={text} onChange={(e) => setText(e.target.value)} placeholder="写下日程、任务、临时安排或一个想法…" className="block min-h-16 w-full resize-none border border-border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-primary/30" /></div>
      <label className="flex h-10 w-10 shrink-0 cursor-pointer items-center justify-center border border-border hover:bg-muted" title="添加截图"><FileImage className="h-4 w-4" /><input ref={fileRef} className="hidden" type="file" accept="image/png,image/jpeg,image/webp,image/gif" onChange={(e) => setAttachment(e.target.files?.[0])} /></label>
      <button disabled={busy || (!text.trim() && !attachment)} className="flex h-10 shrink-0 items-center gap-2 bg-primary px-4 text-sm font-medium text-primary-foreground disabled:opacity-50">{busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}<span className="hidden sm:inline">交给 Agenda</span></button>
    </form>
    {attachment ? <p className="mx-auto mt-2 max-w-5xl text-xs text-muted-foreground">已附加：{attachment.name}</p> : null}
    {error ? <p className="mx-auto mt-2 max-w-5xl text-sm text-destructive">{error}</p> : null}
  </section>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <label className="text-xs text-muted-foreground">{label}{children}</label> }

function CandidateEditor({ value, projects, onChange, onRemove }: { value: Candidate; projects: Project[]; onChange: (value: Candidate) => void; onRemove: () => void }) {
  return <div className="space-y-3 border border-border bg-background p-3">
    <div className="grid gap-2 sm:grid-cols-[110px_1fr_auto]"><select value={value.type} onChange={(e) => onChange({ ...value, type: e.target.value as Candidate["type"] })} className="border border-border bg-background px-2 py-2 text-sm"><option value="task">任务</option><option value="event">日程</option><option value="reminder">提醒</option><option value="note">记录</option></select><input value={value.title} onChange={(e) => onChange({ ...value, title: e.target.value })} className="min-w-0 border border-border bg-background px-3 py-2" placeholder="标题" /><button type="button" onClick={onRemove} className="px-2 text-sm text-muted-foreground hover:text-destructive">移除</button></div>
    <textarea value={value.description ?? ""} onChange={(e) => onChange({ ...value, description: e.target.value })} className="min-h-14 w-full border border-border bg-background px-3 py-2 text-sm" placeholder="补充说明" />
    <div className="grid gap-2 sm:grid-cols-2">
      <Field label="开始时间"><input type="datetime-local" value={localInput(value.startAt)} onChange={(e) => onChange({ ...value, startAt: fromLocalInput(e.target.value) })} className="field" /></Field>
      <Field label="截止时间"><input type="datetime-local" value={localInput(value.dueAt)} onChange={(e) => onChange({ ...value, dueAt: fromLocalInput(e.target.value) })} className="field" /></Field>
      <Field label="提醒时间"><input type="datetime-local" value={localInput(value.reminderAt)} onChange={(e) => onChange({ ...value, reminderAt: fromLocalInput(e.target.value) })} className="field" /></Field>
      <Field label="项目"><input list="agenda-projects" value={value.project ?? ""} onChange={(e) => onChange({ ...value, project: e.target.value })} className="field" /></Field>
      <Field label="确定性"><select value={value.certainty ?? "confirmed"} onChange={(e) => onChange({ ...value, certainty: e.target.value as Candidate["certainty"] })} className="field"><option value="confirmed">正式</option><option value="tentative">暂定</option></select></Field>
      <Field label="预计用时（分钟）"><input type="number" min={0} value={value.durationMinutes ?? 0} onChange={(e) => onChange({ ...value, durationMinutes: Number(e.target.value) })} className="field" /></Field>
    </div>
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
  return <article className="border border-border bg-card">
    <div className="border-b border-border px-4 py-3"><div className="flex items-center justify-between gap-3 text-xs text-muted-foreground"><span className="flex items-center gap-1.5">{capture.hasAttachment ? <FileImage className="h-3.5 w-3.5" /> : <Inbox className="h-3.5 w-3.5" />}{new Date(capture.createdAt).toLocaleString("zh-CN")}</span><span>{capture.status === "needs_review" ? "等待处理" : capture.status}</span></div><p className="mt-2 whitespace-pre-wrap text-sm">{capture.rawText || capture.attachmentName || "图片记录"}</p>{capture.hasAttachment ? <a className="mt-2 inline-block text-xs text-primary hover:underline" href={`/api/organizer/v1/captures/${capture.id}/attachment`} target="_blank" rel="noreferrer">查看原截图</a> : null}</div>
    <div className="space-y-3 p-4">{candidates.map((candidate, index) => <CandidateEditor key={index} value={candidate} projects={projects} onChange={(next) => setCandidates((all) => all.map((item, i) => i === index ? next : item))} onRemove={() => setCandidates((all) => all.filter((_, i) => i !== index))} />)}
      {error || capture.error ? <p className="bg-destructive/10 px-3 py-2 text-sm text-destructive">{error || capture.error}</p> : null}
      {candidates.length === 0 ? <button disabled={busy} onClick={parse} className="flex items-center gap-2 bg-secondary px-3 py-2 text-sm font-medium"><Sparkles className="h-4 w-4" />AI 整理</button> : <button disabled={busy || candidates.some((v) => !v.title.trim())} onClick={confirm} className="flex items-center gap-2 bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50">{busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}确认并安排</button>}
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
  return <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/45 p-0 sm:items-center sm:p-6" onMouseDown={(e) => { if (e.currentTarget === e.target) onClose() }}>
    <section role="dialog" aria-modal="true" aria-labelledby="agenda-item-dialog-title" className="max-h-[92vh] w-full max-w-2xl overflow-y-auto border border-border bg-card shadow-2xl"><header className="sticky top-0 z-10 flex items-center justify-between border-b border-border bg-card px-5 py-4"><div><p className="text-xs text-muted-foreground">{item ? "事项详情" : "手动创建"}</p><h2 id="agenda-item-dialog-title" className="font-semibold">{item ? "编辑事项" : "新增事项"}</h2></div><button onClick={onClose} className="p-2 hover:bg-muted" aria-label="关闭"><X className="h-4 w-4" /></button></header>
      <div className="space-y-4 p-5"><CandidateEditor value={value} projects={projects} onChange={setValue} onRemove={onClose} />{item ? <Field label="状态"><select value={status} onChange={(e) => setStatus(e.target.value as OrganizerItem["status"])} className="field"><option value="todo">待办</option><option value="doing">进行中</option><option value="done">已完成</option><option value="cancelled">已取消</option><option value="archived">已归档</option></select></Field> : null}{error ? <p className="bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p> : null}<div className="flex justify-end gap-2"><button onClick={onClose} className="border border-border px-4 py-2 text-sm">取消</button><button onClick={save} disabled={busy || !value.title.trim()} className="flex items-center gap-2 bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50">{busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}保存</button></div></div>
    </section>
  </div>
}

function ItemRow({ item, onEdit, onChanged, history = false }: { item: OrganizerItem; onEdit: () => void; onChanged: () => Promise<void>; history?: boolean }) {
  const [busy, setBusy] = useState(false)
  async function run(action: () => Promise<unknown>) { setBusy(true); try { await action(); await onChanged() } finally { setBusy(false) } }
  return <article className="group grid grid-cols-[auto_minmax(0,1fr)_auto] gap-3 border-b border-border px-1 py-3 last:border-b-0">
    {!history ? <button aria-label="完成事项" disabled={busy} onClick={() => run(() => organizerApi.completeItem(item.id))} className="mt-0.5 flex h-5 w-5 items-center justify-center rounded-full border border-primary/50 text-primary hover:bg-primary hover:text-primary-foreground">{busy ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3 opacity-0 group-hover:opacity-100" />}</button> : <span className="mt-0.5 text-muted-foreground">{item.status === "done" ? <CheckCircle2 className="h-5 w-5 text-emerald-600" /> : item.status === "cancelled" ? <X className="h-5 w-5" /> : <Archive className="h-5 w-5" />}</span>}
    <button className="min-w-0 text-left" onClick={onEdit}><div className="flex flex-wrap items-center gap-2"><h3 className={`font-medium ${history ? "text-muted-foreground line-through decoration-border" : ""}`}>{item.title}</h3>{item.certainty === "tentative" ? <span className="border border-amber-500/50 px-1.5 py-0.5 text-[11px] text-amber-700">暂定</span> : null}{(item.priority ?? 0) > 0 ? <span className="text-xs font-medium text-orange-600">P{item.priority}</span> : null}</div>{item.description ? <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">{item.description}</p> : null}<div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">{itemMoment(item) ? <span className="flex items-center gap-1"><Clock3 className="h-3 w-3" />{displayTime(itemMoment(item), true)}</span> : <span>未安排时间</span>}{item.project ? <span className="font-medium text-foreground/70">{item.project}</span> : null}{item.durationMinutes ? <span>{item.durationMinutes} 分钟</span> : null}</div></button>
    <div className="flex items-start gap-1">{history ? <button disabled={busy} onClick={() => run(() => organizerApi.reopenItem(item.id))} className="p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground" title="重新打开"><RotateCcw className="h-4 w-4" /></button> : <><button onClick={onEdit} className="p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground" title="编辑"><Edit3 className="h-4 w-4" /></button><button disabled={busy} onClick={() => run(() => organizerApi.cancelItem(item.id))} className="p-1.5 text-muted-foreground hover:bg-muted hover:text-destructive" title="取消"><X className="h-4 w-4" /></button></>}</div>
  </article>
}

function ItemList({ title, items, onEdit, onChanged, history }: { title?: string; items: OrganizerItem[]; onEdit: (item: OrganizerItem) => void; onChanged: () => Promise<void>; history?: boolean }) {
  if (!items.length) return null
  return <section>{title ? <div className="mb-2 flex items-center gap-2"><h2 className="text-sm font-semibold">{title}</h2><span className="text-xs text-muted-foreground">{items.length}</span></div> : null}<div className="border-y border-border bg-card px-3">{items.map((item) => <ItemRow key={item.id} item={item} onEdit={() => onEdit(item)} onChanged={onChanged} history={history} />)}</div></section>
}

function ProjectEditor({ onClose, onSaved }: { onClose: () => void; onSaved: () => Promise<void> }) {
  const [name, setName] = useState(""), [goal, setGoal] = useState(""), [busy, setBusy] = useState(false)
  async function save() { setBusy(true); try { await organizerApi.createProject({ name, goal, status: "active", priority: 0 }); await onSaved(); onClose() } finally { setBusy(false) } }
  return <div className="flex gap-2 border border-border bg-card p-3"><input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="项目名称" className="min-w-0 flex-1 border border-border bg-background px-3 py-2 text-sm" /><input value={goal} onChange={(e) => setGoal(e.target.value)} placeholder="项目目标（可选）" className="hidden min-w-0 flex-[2] border border-border bg-background px-3 py-2 text-sm sm:block" /><button onClick={save} disabled={busy || !name.trim()} className="bg-primary px-3 text-sm text-primary-foreground">创建</button><button onClick={onClose} className="p-2"><X className="h-4 w-4" /></button></div>
}

function SettingsPanel({ onClose, onMessage }: { onClose: () => void; onMessage: (value: string) => void }) {
  const [deviceToken, setDeviceToken] = useState(""), [currentPassword, setCurrentPassword] = useState(""), [newPassword, setNewPassword] = useState(""), [busy, setBusy] = useState(false)
  async function token() { const value = await organizerApi.createDeviceToken("Obsidian"); setDeviceToken(value.token) }
  async function password(event: React.FormEvent) { event.preventDefault(); setBusy(true); try { await organizerApi.changePassword(currentPassword, newPassword); setCurrentPassword(""); setNewPassword(""); onMessage("密码已修改。") } finally { setBusy(false) } }
  return <div className="fixed inset-0 z-50 flex justify-end bg-black/40" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose() }}><aside className="h-full w-full max-w-md overflow-y-auto border-l border-border bg-card p-6 shadow-2xl"><header className="mb-8 flex items-center justify-between"><h2 className="text-lg font-semibold">设置</h2><button onClick={onClose} className="p-2 hover:bg-muted"><X className="h-4 w-4" /></button></header><section className="space-y-3"><h3 className="font-medium">通知与设备</h3><button onClick={token} className="border border-border px-3 py-2 text-sm">生成 Obsidian 令牌</button>{deviceToken ? <div className="break-all bg-muted p-3 font-mono text-xs select-all">{deviceToken}</div> : null}<a className="block text-sm text-primary hover:underline" href="/api/organizer/v1/export">导出完整数据</a></section><form onSubmit={password} className="mt-8 space-y-3 border-t border-border pt-6"><h3 className="font-medium">修改密码</h3><input type="password" value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} placeholder="当前密码" className="field" required /><input type="password" minLength={12} value={newPassword} onChange={(e) => setNewPassword(e.target.value)} placeholder="新密码（至少 12 位）" className="field" required /><button disabled={busy} className="bg-secondary px-3 py-2 text-sm font-medium">修改密码</button></form></aside></div>
}

export function AgendaApp() {
  const [session, setSession] = useState<Session | null>(null), [authChecked, setAuthChecked] = useState(false)
  const [items, setItems] = useState<OrganizerItem[]>([]), [captures, setCaptures] = useState<Capture[]>([]), [projects, setProjects] = useState<Project[]>([]), [memories, setMemories] = useState<Memory[]>([]), [events, setEvents] = useState<ItemEvent[]>([])
  const [view, setView] = useState<View>("today"), [search, setSearch] = useState(""), [editing, setEditing] = useState<OrganizerItem | "new" | null>(null), [settingsOpen, setSettingsOpen] = useState(false), [newProject, setNewProject] = useState(false)
  const [error, setError] = useState(""), [message, setMessage] = useState("")

  const refresh = useCallback(async () => {
    const [itemResponse, captureResponse, projectResponse, memoryResponse, eventResponse] = await Promise.all([organizerApi.listItems(), organizerApi.listCaptures(), organizerApi.listProjects(), organizerApi.listMemories(), organizerApi.listEvents()])
    setItems(itemResponse.items); setCaptures(captureResponse.captures); setProjects(projectResponse.projects); setMemories(memoryResponse.memories); setEvents(eventResponse.events)
  }, [])
  const checkSession = useCallback(async () => { try { setSession(await organizerApi.session()); await refresh() } catch (e) { if (e instanceof OrganizerApiError && e.status === 401) setSession(null); else setError(errorMessage(e)) } finally { setAuthChecked(true) } }, [refresh])
  useEffect(() => { void checkSession() }, [checkSession])
  useEffect(() => { if ("serviceWorker" in navigator) void navigator.serviceWorker.register("/agenda/sw.js", { scope: "/agenda/" }) }, [])

  const filtered = useMemo(() => { const q = search.trim().toLocaleLowerCase(); return q ? items.filter((item) => [item.title, item.description, item.project, item.location, ...(item.tags ?? [])].some((value) => value?.toLocaleLowerCase().includes(q))) : items }, [items, search])
  const active = filtered.filter(isActive), history = filtered.filter((item) => !isActive(item))
  const pending = captures.filter((capture) => capture.status !== "confirmed")
  const todayItems = active.filter((item) => itemMoment(item) && sameDay(itemMoment(item)))
  const overdue = active.filter((item) => { const moment = itemMoment(item); return moment && new Date(moment) < new Date(new Date().setHours(0, 0, 0, 0)) })
  const unscheduled = active.filter((item) => !itemMoment(item))
  const calendarGroups = useMemo(() => { const result = new Map<string, OrganizerItem[]>(); for (const item of active) { const key = itemMoment(item) ? dayKey(itemMoment(item)) : "未安排"; result.set(key, [...(result.get(key) ?? []), item]) } return [...result.entries()] }, [active])

  async function enablePush() {
    try { if (!("serviceWorker" in navigator) || !("PushManager" in window)) throw new Error("当前浏览器不支持通知"); const permission = await Notification.requestPermission(); if (permission !== "granted") throw new Error("通知权限没有开启"); const registration = await navigator.serviceWorker.ready; const vapid = await organizerApi.vapidKey(); if (!vapid.enabled) throw new Error("服务器尚未配置推送"); let subscription = await registration.pushManager.getSubscription(); subscription ??= await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: urlBase64ToUint8Array(vapid.publicKey) }); await organizerApi.subscribe(subscription.toJSON()); setMessage("这台设备已经开启提醒。") } catch (e) { setError(errorMessage(e)) }
  }
  async function updateMemory(memory: Memory, status: Memory["status"]) { try { await organizerApi.updateMemory({ ...memory, status }); await refresh() } catch (e) { setError(errorMessage(e)) } }
  async function createMemory() { const content = window.prompt("写下希望 Agenda 记住的规则"); if (!content?.trim()) return; await organizerApi.createMemory({ content: content.trim(), kind: "preference", status: "active", scope: "global" }); await refresh() }

  if (!authChecked) return <div className="flex min-h-screen items-center justify-center"><Loader2 className="h-7 w-7 animate-spin text-primary" /></div>
  if (!session) return <Login onAuthenticated={() => void checkSession()} />
  const title = navItems.find((item) => item.id === view)?.label ?? "Agenda"

  return <main className="min-h-screen bg-background text-foreground">
    <div className="min-h-screen md:grid md:grid-cols-[216px_minmax(0,1fr)]">
      <aside className="hidden border-r border-border bg-card md:flex md:flex-col"><div className="border-b border-border px-5 py-5"><p className="text-xs text-muted-foreground">个人工作台</p><h1 className="mt-1 text-xl font-semibold">Agenda</h1></div><nav className="flex-1 space-y-1 p-3">{navItems.map(({ id, label, icon: Icon }) => <button key={id} onClick={() => setView(id)} className={`flex w-full items-center gap-3 px-3 py-2 text-sm ${view === id ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}><Icon className="h-4 w-4" />{label}{id === "inbox" && pending.length ? <span className="ml-auto text-xs">{pending.length}</span> : null}{id === "memory" && memories.some((m) => m.status === "proposed") ? <CircleDashed className="ml-auto h-3.5 w-3.5" /> : null}</button>)}</nav><div className="border-t border-border p-3"><button onClick={() => setSettingsOpen(true)} className="flex w-full items-center gap-3 px-3 py-2 text-sm text-muted-foreground hover:bg-muted hover:text-foreground"><Settings className="h-4 w-4" />设置</button></div></aside>
      <div className="min-w-0 pb-20 md:pb-0"><header className="flex h-16 items-center justify-between border-b border-border bg-card px-4 sm:px-6"><div className="flex items-center gap-3"><Menu className="h-5 w-5 md:hidden" /><div><h1 className="font-semibold">{title}</h1><p className="hidden text-xs text-muted-foreground sm:block">{view === "today" ? new Intl.DateTimeFormat("zh-CN", { month: "long", day: "numeric", weekday: "long" }).format(new Date()) : "管理你的日程、项目与历史"}</p></div></div><div className="flex items-center gap-1"><button onClick={() => setEditing("new")} className="flex items-center gap-2 bg-primary px-3 py-2 text-sm text-primary-foreground"><Plus className="h-4 w-4" /><span className="hidden sm:inline">新增</span></button><button onClick={() => void refresh()} className="p-2.5 hover:bg-muted" aria-label="刷新"><RefreshCw className="h-4 w-4" /></button><button onClick={enablePush} className="p-2.5 hover:bg-muted" aria-label="通知"><Bell className="h-4 w-4" /></button><button onClick={() => setSettingsOpen(true)} className="p-2.5 hover:bg-muted md:hidden" aria-label="设置"><Settings className="h-4 w-4" /></button><button onClick={async () => { await organizerApi.logout(); setSession(null) }} className="p-2.5 hover:bg-muted" aria-label="退出"><LogOut className="h-4 w-4" /></button></div></header>
        <QuickCapture onCreated={async () => { await refresh(); setView("inbox") }} />
        <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6">
          {(view === "calendar" || view === "projects" || view === "history") ? <div className="mb-5 flex items-center gap-2 border border-border bg-card px-3"><Search className="h-4 w-4 text-muted-foreground" /><input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="搜索标题、项目、标签或地点" className="h-10 min-w-0 flex-1 bg-transparent text-sm outline-none" />{search ? <button onClick={() => setSearch("")}><X className="h-4 w-4" /></button> : null}</div> : null}
          {error ? <div className="mb-4 bg-destructive/10 px-4 py-3 text-sm text-destructive">{error}<button onClick={() => setError("")} className="float-right"><X className="h-4 w-4" /></button></div> : null}{message ? <div className="mb-4 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-700">{message}</div> : null}

          {view === "today" ? <div className="space-y-7"><section className="grid border border-border bg-card sm:grid-cols-4">{[["今天", todayItems.length], ["逾期", overdue.length], ["待整理", pending.length], ["活跃项目", projects.filter((p) => p.status === "active").length]].map(([label, count], index) => <div key={label} className={`px-4 py-3 ${index ? "border-t border-border sm:border-l sm:border-t-0" : ""}`}><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 text-2xl font-semibold">{count}</p></div>)}</section><ItemList title="已逾期" items={overdue} onEdit={setEditing} onChanged={refresh} /><ItemList title="今天" items={todayItems.filter((item) => !overdue.includes(item))} onEdit={setEditing} onChanged={refresh} /><ItemList title="下一步行动" items={unscheduled.slice(0, 8)} onEdit={setEditing} onChanged={refresh} />{!todayItems.length && !overdue.length && !unscheduled.length ? <Empty text="今天没有待处理事项。" /> : null}{history.filter((item) => item.status === "done" && item.completedAt && sameDay(item.completedAt)).length ? <details><summary className="cursor-pointer text-sm font-medium text-muted-foreground">今天已完成 {history.filter((item) => item.status === "done" && item.completedAt && sameDay(item.completedAt)).length} 项</summary><div className="mt-2"><ItemList items={history.filter((item) => item.status === "done" && item.completedAt && sameDay(item.completedAt))} onEdit={setEditing} onChanged={refresh} history /></div></details> : null}</div> : null}

          {view === "calendar" ? <div className="space-y-7">{calendarGroups.map(([date, values]) => <ItemList key={date} title={date} items={values} onEdit={setEditing} onChanged={refresh} />)}{!active.length ? <Empty text="没有进行中的日程或任务。" /> : null}</div> : null}

          {view === "projects" ? <div className="space-y-4"><div className="flex items-center justify-between"><p className="text-sm text-muted-foreground">每个项目都保留当前行动和完整历史。</p><button onClick={() => setNewProject(true)} className="flex items-center gap-2 border border-border px-3 py-2 text-sm hover:bg-muted"><Plus className="h-4 w-4" />新项目</button></div>{newProject ? <ProjectEditor onClose={() => setNewProject(false)} onSaved={refresh} /> : null}<div className="grid gap-4 lg:grid-cols-2">{projects.map((project) => { const projectItems = filtered.filter((item) => item.projectId === project.id || item.project === project.name); return <section key={project.id} className="border border-border bg-card"><header className="border-b border-border px-4 py-3"><div className="flex items-center justify-between gap-3"><div><h2 className="font-semibold">{project.name}</h2><p className="mt-0.5 text-xs text-muted-foreground">{project.status === "active" ? "进行中" : project.status} · {project.openCount} 项待处理 · {project.doneCount} 项完成</p></div><ChevronRight className="h-4 w-4 text-muted-foreground" /></div>{project.goal ? <p className="mt-2 text-sm text-muted-foreground">{project.goal}</p> : null}</header><div className="px-3">{projectItems.filter(isActive).slice(0, 5).map((item) => <ItemRow key={item.id} item={item} onEdit={() => setEditing(item)} onChanged={refresh} />)}{!projectItems.filter(isActive).length ? <p className="py-5 text-center text-sm text-muted-foreground">还没有下一步行动</p> : null}</div></section>})}</div>{!projects.length ? <Empty text="还没有项目。创建项目后，Agenda 会按项目汇总工作。" /> : null}</div> : null}

          {view === "inbox" ? <div className="space-y-4"><div><h2 className="font-semibold">等待你整理</h2><p className="mt-1 text-sm text-muted-foreground">这里保存刚记录的内容和 AI 建议。确认后会进入日程或项目，但原始内容仍会保留。</p></div>{pending.map((capture) => <CaptureCard key={`${capture.id}-${capture.updatedAt}`} capture={capture} projects={projects} onChanged={refresh} />)}{!pending.length ? <Empty text="没有等待整理的内容。" /> : null}</div> : null}

          {view === "history" ? <div className="space-y-7"><section className="flex gap-5 border-b border-border pb-4 text-sm text-muted-foreground"><span>完成 {history.filter((i) => i.status === "done").length}</span><span>取消 {history.filter((i) => i.status === "cancelled").length}</span><span>归档 {history.filter((i) => i.status === "archived").length}</span><span>活动记录 {events.length}</span></section><ItemList title="已完成" items={history.filter((i) => i.status === "done").sort((a, b) => (b.completedAt ?? b.updatedAt).localeCompare(a.completedAt ?? a.updatedAt))} onEdit={setEditing} onChanged={refresh} history /><ItemList title="已取消" items={history.filter((i) => i.status === "cancelled")} onEdit={setEditing} onChanged={refresh} history /><ItemList title="已归档" items={history.filter((i) => i.status === "archived")} onEdit={setEditing} onChanged={refresh} history />{!history.length ? <Empty text="完成、取消和归档的内容会保留在这里。" /> : null}</div> : null}

          {view === "memory" ? <div className="space-y-6"><div className="flex items-start justify-between gap-4"><div><h2 className="font-semibold">Agenda 记住的规则</h2><p className="mt-1 max-w-2xl text-sm text-muted-foreground">只有已批准规则会参与后续解析。重复纠正形成的规律会先等待你确认。</p></div><button onClick={() => void createMemory()} className="flex shrink-0 items-center gap-2 border border-border px-3 py-2 text-sm hover:bg-muted"><Plus className="h-4 w-4" />添加规则</button></div>{memories.filter((m) => m.status === "proposed").length ? <section><h3 className="mb-2 text-sm font-semibold">待你决定</h3><div className="divide-y divide-border border border-border bg-card">{memories.filter((m) => m.status === "proposed").map((memory) => <div key={memory.id} className="p-4"><p className="text-sm">{memory.content}</p><p className="mt-1 text-xs text-muted-foreground">来自 {memory.evidenceCount} 次相似纠正</p><div className="mt-3 flex gap-2"><button onClick={() => void updateMemory(memory, "active")} className="bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground">记住</button><button onClick={() => void updateMemory(memory, "dismissed")} className="border border-border px-3 py-1.5 text-xs">忽略</button></div></div>)}</div></section> : null}<section><h3 className="mb-2 text-sm font-semibold">正在使用</h3><div className="divide-y divide-border border border-border bg-card">{memories.filter((m) => m.status === "active").map((memory) => <div key={memory.id} className="flex items-start justify-between gap-4 p-4"><div><p className="text-sm">{memory.content}</p><p className="mt-1 text-xs text-muted-foreground">{memory.kind === "procedure" ? "助手规则" : "个人偏好"} · {memory.scope}</p></div><button onClick={() => void updateMemory(memory, "forgotten")} className="p-2 text-muted-foreground hover:bg-muted hover:text-destructive" title="忘记"><Trash2 className="h-4 w-4" /></button></div>)}{!memories.some((m) => m.status === "active") ? <p className="p-6 text-center text-sm text-muted-foreground">还没有已批准的个人规则。</p> : null}</div></section></div> : null}
        </div>
      </div>
    </div>
    <nav className="fixed inset-x-0 bottom-0 z-30 grid grid-cols-6 border-t border-border bg-card md:hidden">{navItems.map(({ id, label, icon: Icon }) => <button key={id} onClick={() => setView(id)} className={`flex h-16 flex-col items-center justify-center gap-1 text-[10px] ${view === id ? "text-primary" : "text-muted-foreground"}`}><Icon className="h-4 w-4" />{label}</button>)}</nav>
    {editing ? <ItemDialog item={editing === "new" ? undefined : editing} projects={projects} onClose={() => setEditing(null)} onSaved={refresh} /> : null}
    {settingsOpen ? <SettingsPanel onClose={() => setSettingsOpen(false)} onMessage={setMessage} /> : null}
  </main>
}

function Empty({ text }: { text: string }) { return <div className="border border-dashed border-border px-5 py-10 text-center text-sm text-muted-foreground">{text}</div> }

function urlBase64ToUint8Array(value: string) {
  const padding = "=".repeat((4 - value.length % 4) % 4), base64 = (value + padding).replace(/-/g, "+").replace(/_/g, "/"), raw = window.atob(base64)
  return Uint8Array.from([...raw].map((char) => char.charCodeAt(0)))
}
