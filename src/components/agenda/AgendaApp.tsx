import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import {
  Bell,
  CalendarDays,
  Check,
  Clock3,
  FileImage,
  Inbox,
  KeyRound,
  Loader2,
  LogOut,
  RefreshCw,
  Send,
  Settings,
  Sparkles,
} from "lucide-react"
import {
  organizerApi,
  OrganizerApiError,
  type Candidate,
  type Capture,
  type OrganizerItem,
} from "@/lib/organizer-api"

type Session = { authenticated: boolean; pushEnabled: boolean; timezone: string }

function errorMessage(error: unknown) {
  if (error instanceof OrganizerApiError) return error.message
  if (error instanceof Error) return error.message
  return "发生未知错误"
}

function displayTime(value?: string) {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat("zh-CN", {
    month: "numeric",
    day: "numeric",
    weekday: "short",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
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

function itemMoment(item: OrganizerItem) {
  return item.startAt || item.dueAt || item.reminderAt || ""
}

function startOfLocalDay(date = new Date()) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate())
}

function groupItems(items: OrganizerItem[]) {
  const start = startOfLocalDay()
  const tomorrow = new Date(start)
  tomorrow.setDate(tomorrow.getDate() + 1)
  const afterTomorrow = new Date(tomorrow)
  afterTomorrow.setDate(afterTomorrow.getDate() + 1)

  const active = items.filter((item) => item.status !== "done" && item.status !== "cancelled")
  const groups: Record<"overdue" | "today" | "tomorrow" | "later" | "unscheduled", OrganizerItem[]> = {
    overdue: [],
    today: [],
    tomorrow: [],
    later: [],
    unscheduled: [],
  }
  for (const item of active) {
    const moment = itemMoment(item)
    if (!moment) {
      groups.unscheduled.push(item)
      continue
    }
    const date = new Date(moment)
    if (date < start) groups.overdue.push(item)
    else if (date < tomorrow) groups.today.push(item)
    else if (date < afterTomorrow) groups.tomorrow.push(item)
    else groups.later.push(item)
  }
  return groups
}

function Login({ onAuthenticated }: { onAuthenticated: () => void }) {
  const [password, setPassword] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError("")
    try {
      await organizerApi.login(password)
      setPassword("")
      onAuthenticated()
    } catch (caught) {
      setError(errorMessage(caught))
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="min-h-screen bg-background px-5 py-16 text-foreground">
      <div className="mx-auto max-w-md rounded-3xl border border-border bg-card p-7 shadow-xl">
        <div className="mb-7 flex items-center gap-3">
          <div className="rounded-2xl bg-primary/10 p-3 text-primary"><KeyRound className="h-6 w-6" /></div>
          <div>
            <p className="text-sm text-muted-foreground">Faber 的私人空间</p>
            <h1 className="text-2xl font-semibold">一个闪念 · 日程</h1>
          </div>
        </div>
        <form className="space-y-4" onSubmit={submit}>
          <label className="block text-sm font-medium" htmlFor="organizer-password">登录密码</label>
          <input
            id="organizer-password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            className="w-full rounded-xl border border-border bg-background px-4 py-3 outline-none ring-primary/30 focus:ring-2"
            required
          />
          {error ? <p className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p> : null}
          <button disabled={busy} className="flex w-full items-center justify-center gap-2 rounded-xl bg-primary px-4 py-3 font-medium text-primary-foreground disabled:opacity-60">
            {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <KeyRound className="h-4 w-4" />}
            登录
          </button>
        </form>
        <a href="/" className="mt-6 block text-center text-sm text-muted-foreground hover:text-foreground">返回博客</a>
      </div>
    </main>
  )
}

function CandidateEditor({ value, onChange, onRemove }: { value: Candidate; onChange: (value: Candidate) => void; onRemove: () => void }) {
  return (
    <div className="space-y-3 rounded-2xl border border-border bg-background/60 p-4">
      <div className="flex gap-2">
        <select value={value.type} onChange={(event) => onChange({ ...value, type: event.target.value as Candidate["type"] })} className="rounded-lg border border-border bg-background px-2 py-2 text-sm">
          <option value="task">任务</option>
          <option value="event">日程</option>
          <option value="reminder">提醒</option>
        </select>
        <input value={value.title} onChange={(event) => onChange({ ...value, title: event.target.value })} className="min-w-0 flex-1 rounded-lg border border-border bg-background px-3 py-2" placeholder="事项标题" />
        <button type="button" onClick={onRemove} className="rounded-lg px-2 text-sm text-muted-foreground hover:bg-muted">移除</button>
      </div>
      <textarea value={value.description ?? ""} onChange={(event) => onChange({ ...value, description: event.target.value })} className="min-h-16 w-full rounded-lg border border-border bg-background px-3 py-2 text-sm" placeholder="说明（可选）" />
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="text-xs text-muted-foreground">开始时间
          <input type="datetime-local" value={localInput(value.startAt)} onChange={(event) => onChange({ ...value, startAt: fromLocalInput(event.target.value) })} className="mt-1 w-full rounded-lg border border-border bg-background px-3 py-2 text-sm text-foreground" />
        </label>
        <label className="text-xs text-muted-foreground">截止时间
          <input type="datetime-local" value={localInput(value.dueAt)} onChange={(event) => onChange({ ...value, dueAt: fromLocalInput(event.target.value) })} className="mt-1 w-full rounded-lg border border-border bg-background px-3 py-2 text-sm text-foreground" />
        </label>
        <label className="text-xs text-muted-foreground">提醒时间
          <input type="datetime-local" value={localInput(value.reminderAt)} onChange={(event) => onChange({ ...value, reminderAt: fromLocalInput(event.target.value) })} className="mt-1 w-full rounded-lg border border-border bg-background px-3 py-2 text-sm text-foreground" />
        </label>
        <label className="text-xs text-muted-foreground">地点
          <input value={value.location ?? ""} onChange={(event) => onChange({ ...value, location: event.target.value })} className="mt-1 w-full rounded-lg border border-border bg-background px-3 py-2 text-sm text-foreground" />
        </label>
      </div>
      {value.ambiguities?.length ? (
        <div className="rounded-lg bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
          {value.ambiguities.join("；")}
        </div>
      ) : null}
    </div>
  )
}

function CaptureCard({ capture, onChanged }: { capture: Capture; onChanged: () => Promise<void> }) {
  const [candidates, setCandidates] = useState<Candidate[]>(capture.aiResult?.items ?? [])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  async function parse() {
    setBusy(true)
    setError("")
    try {
      const result = await organizerApi.parseCapture(capture.id)
      setCandidates(result.items)
      await onChanged()
    } catch (caught) {
      setError(errorMessage(caught))
    } finally {
      setBusy(false)
    }
  }

  async function confirm() {
    setBusy(true)
    setError("")
    try {
      await organizerApi.confirmCapture(capture.id, candidates)
      await onChanged()
    } catch (caught) {
      setError(errorMessage(caught))
    } finally {
      setBusy(false)
    }
  }

  return (
    <article className="space-y-4 rounded-2xl border border-border bg-card p-4 shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="mb-1 flex items-center gap-2 text-xs text-muted-foreground">
            {capture.hasAttachment ? <FileImage className="h-3.5 w-3.5" /> : <Inbox className="h-3.5 w-3.5" />}
            {new Date(capture.createdAt).toLocaleString("zh-CN")}
          </div>
          <p className="whitespace-pre-wrap text-sm">{capture.rawText || capture.attachmentName || "图片事项"}</p>
        </div>
        <span className="rounded-full bg-secondary px-2.5 py-1 text-xs text-secondary-foreground">{capture.status}</span>
      </div>
      {capture.hasAttachment ? <a className="text-xs text-primary hover:underline" href={`/api/organizer/v1/captures/${capture.id}/attachment`} target="_blank" rel="noreferrer">查看原截图</a> : null}
      {candidates.map((candidate, index) => (
        <CandidateEditor
          key={index}
          value={candidate}
          onChange={(next) => setCandidates((current) => current.map((item, currentIndex) => currentIndex === index ? next : item))}
          onRemove={() => setCandidates((current) => current.filter((_, currentIndex) => currentIndex !== index))}
        />
      ))}
      {error || capture.error ? <p className="rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">{error || capture.error}</p> : null}
      <div className="flex flex-wrap gap-2">
        {capture.status !== "confirmed" && candidates.length === 0 ? (
          <button type="button" disabled={busy} onClick={parse} className="flex items-center gap-2 rounded-xl bg-secondary px-3 py-2 text-sm font-medium disabled:opacity-50">
            {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Sparkles className="h-4 w-4" />} AI 整理
          </button>
        ) : null}
        {capture.status !== "confirmed" && candidates.length > 0 ? (
          <button type="button" disabled={busy || candidates.some((candidate) => !candidate.title.trim())} onClick={confirm} className="flex items-center gap-2 rounded-xl bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50">
            {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />} 确认加入日程
          </button>
        ) : null}
      </div>
    </article>
  )
}

function ItemRow({ item, onChanged }: { item: OrganizerItem; onChanged: () => Promise<void> }) {
  const [busy, setBusy] = useState(false)
  async function run(action: () => Promise<unknown>) {
    setBusy(true)
    try {
      await action()
      await onChanged()
    } finally {
      setBusy(false)
    }
  }
  return (
    <article className="group flex items-start gap-3 rounded-2xl border border-border bg-card p-4 shadow-sm">
      <button aria-label="完成事项" disabled={busy} onClick={() => run(() => organizerApi.completeItem(item.id))} className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full border border-primary/40 text-primary hover:bg-primary hover:text-primary-foreground disabled:opacity-50">
        {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5 opacity-0 group-hover:opacity-100" />}
      </button>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="font-medium">{item.title}</h3>
          {(item.priority ?? 0) > 0 ? <span className="rounded-full bg-orange-500/10 px-2 py-0.5 text-xs text-orange-600">P{item.priority}</span> : null}
        </div>
        {item.description ? <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">{item.description}</p> : null}
        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
          {itemMoment(item) ? <span className="flex items-center gap-1"><Clock3 className="h-3.5 w-3.5" />{displayTime(itemMoment(item))}</span> : null}
          {item.location ? <span>{item.location}</span> : null}
          {item.project ? <span>#{item.project}</span> : null}
        </div>
      </div>
      <button type="button" disabled={busy} onClick={() => run(() => organizerApi.snoozeItem(item.id, 10))} className="rounded-lg px-2 py-1 text-xs text-muted-foreground hover:bg-muted" title="推迟 10 分钟">稍后</button>
    </article>
  )
}

function ItemSection({ title, items, tone, onChanged }: { title: string; items: OrganizerItem[]; tone?: string; onChanged: () => Promise<void> }) {
  if (!items.length) return null
  return (
    <section className="space-y-3">
      <div className="flex items-center gap-2">
        <h2 className={`text-sm font-semibold ${tone ?? ""}`}>{title}</h2>
        <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">{items.length}</span>
      </div>
      <div className="space-y-2">{items.map((item) => <ItemRow key={item.id} item={item} onChanged={onChanged} />)}</div>
    </section>
  )
}

function urlBase64ToUint8Array(value: string) {
  const padding = "=".repeat((4 - value.length % 4) % 4)
  const base64 = (value + padding).replace(/-/g, "+").replace(/_/g, "/")
  const raw = window.atob(base64)
  return Uint8Array.from([...raw].map((char) => char.charCodeAt(0)))
}

export function AgendaApp() {
  const [session, setSession] = useState<Session | null>(null)
  const [authChecked, setAuthChecked] = useState(false)
  const [items, setItems] = useState<OrganizerItem[]>([])
  const [captures, setCaptures] = useState<Capture[]>([])
  const [text, setText] = useState("")
  const [attachment, setAttachment] = useState<File | undefined>()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [message, setMessage] = useState("")
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [deviceToken, setDeviceToken] = useState("")
  const fileRef = useRef<HTMLInputElement>(null)

  const refresh = useCallback(async () => {
    const [itemResponse, captureResponse] = await Promise.all([organizerApi.listItems(), organizerApi.listCaptures()])
    setItems(itemResponse.items)
    setCaptures(captureResponse.captures)
  }, [])

  const checkSession = useCallback(async () => {
    try {
      const value = await organizerApi.session()
      setSession(value)
      await refresh()
    } catch (caught) {
      if (caught instanceof OrganizerApiError && caught.status === 401) setSession(null)
      else setError(errorMessage(caught))
    } finally {
      setAuthChecked(true)
    }
  }, [refresh])

  useEffect(() => { void checkSession() }, [checkSession])
  useEffect(() => {
    if ("serviceWorker" in navigator) void navigator.serviceWorker.register("/agenda/sw.js", { scope: "/agenda/" })
  }, [])

  const pendingCaptures = captures.filter((capture) => capture.status !== "confirmed")
  const grouped = useMemo(() => groupItems(items), [items])

  async function submitCapture(event: React.FormEvent) {
    event.preventDefault()
    if (!text.trim() && !attachment) return
    setBusy(true)
    setError("")
    setMessage("")
    try {
      const capture = await organizerApi.createCapture(text.trim(), attachment)
      setText("")
      setAttachment(undefined)
      if (fileRef.current) fileRef.current.value = ""
      await organizerApi.parseCapture(capture.id)
      await refresh()
      setMessage("已经整理到待确认收件箱。")
    } catch (caught) {
      setError(errorMessage(caught))
      await refresh().catch(() => undefined)
    } finally {
      setBusy(false)
    }
  }

  async function enablePush() {
    setError("")
    try {
      if (!("serviceWorker" in navigator) || !("PushManager" in window)) throw new Error("当前浏览器不支持 Web Push")
      const permission = await Notification.requestPermission()
      if (permission !== "granted") throw new Error("通知权限没有开启")
      const registration = await navigator.serviceWorker.ready
      const vapid = await organizerApi.vapidKey()
      if (!vapid.enabled) throw new Error("服务器尚未配置推送密钥")
      let subscription = await registration.pushManager.getSubscription()
      subscription ??= await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: urlBase64ToUint8Array(vapid.publicKey) })
      await organizerApi.subscribe(subscription.toJSON())
      setMessage("这台设备已经开启事项提醒。")
    } catch (caught) {
      setError(errorMessage(caught))
    }
  }

  async function createDeviceToken() {
    setError("")
    try {
      const value = await organizerApi.createDeviceToken("Obsidian")
      setDeviceToken(value.token)
    } catch (caught) {
      setError(errorMessage(caught))
    }
  }

  if (!authChecked) {
    return <div className="flex min-h-screen items-center justify-center"><Loader2 className="h-7 w-7 animate-spin text-primary" /></div>
  }
  if (!session) return <Login onAuthenticated={() => void checkSession()} />

  return (
    <main className="min-h-screen bg-background px-4 pb-20 pt-6 text-foreground sm:px-6">
      <div className="mx-auto max-w-5xl">
        <header className="mb-8 flex flex-wrap items-center justify-between gap-4">
          <div>
            <a href="/" className="text-sm text-muted-foreground hover:text-foreground">Faber 的 Blog</a>
            <h1 className="mt-1 flex items-center gap-2 text-2xl font-semibold sm:text-3xl"><CalendarDays className="h-7 w-7 text-primary" />一个闪念 · 日程</h1>
          </div>
          <div className="flex items-center gap-2">
            <button onClick={() => void refresh()} className="rounded-xl border border-border p-2.5 hover:bg-muted" aria-label="刷新"><RefreshCw className="h-4 w-4" /></button>
            <button onClick={enablePush} className="flex items-center gap-2 rounded-xl border border-border px-3 py-2 text-sm hover:bg-muted"><Bell className="h-4 w-4" />通知</button>
            <button onClick={() => setSettingsOpen((value) => !value)} className="rounded-xl border border-border p-2.5 hover:bg-muted" aria-label="设置"><Settings className="h-4 w-4" /></button>
            <button onClick={async () => { await organizerApi.logout(); setSession(null) }} className="rounded-xl border border-border p-2.5 hover:bg-muted" aria-label="退出"><LogOut className="h-4 w-4" /></button>
          </div>
        </header>

        {settingsOpen ? (
          <section className="mb-6 rounded-2xl border border-border bg-card p-5">
            <h2 className="mb-2 font-semibold">设备与 Obsidian</h2>
            <p className="mb-3 text-sm text-muted-foreground">生成一个只供 Obsidian 使用的设备令牌。令牌只显示一次，不要发送给他人。</p>
            <button onClick={createDeviceToken} className="rounded-xl bg-secondary px-3 py-2 text-sm font-medium">生成 Obsidian 令牌</button>
            {deviceToken ? <div className="mt-3 rounded-xl bg-muted p-3 font-mono text-xs break-all select-all">{deviceToken}</div> : null}
            <a className="mt-4 inline-block text-sm text-primary hover:underline" href="/api/organizer/v1/export">导出全部事项数据</a>
          </section>
        ) : null}

        <section className="mb-8 rounded-3xl border border-primary/20 bg-gradient-to-br from-primary/10 via-card to-card p-5 shadow-sm sm:p-7">
          <div className="mb-4 flex items-center gap-2"><Sparkles className="h-5 w-5 text-primary" /><h2 className="font-semibold">把想到的事情交给我整理</h2></div>
          <form onSubmit={submitCapture} className="space-y-3">
            <textarea
              value={text}
              onChange={(event) => setText(event.target.value)}
              rows={3}
              inputMode="text"
              placeholder="例如：明天下午三点给张老师打电话，提前半小时提醒。可以直接使用手机键盘的语音输入。"
              className="w-full resize-y rounded-2xl border border-border bg-background/80 px-4 py-3 outline-none ring-primary/30 focus:ring-2"
            />
            <div className="flex flex-wrap items-center justify-between gap-3">
              <label className="flex cursor-pointer items-center gap-2 rounded-xl border border-border bg-background px-3 py-2 text-sm hover:bg-muted">
                <FileImage className="h-4 w-4" />{attachment ? attachment.name : "添加截图"}
                <input ref={fileRef} className="hidden" type="file" accept="image/png,image/jpeg,image/webp,image/gif" onChange={(event) => setAttachment(event.target.files?.[0])} />
              </label>
              <button disabled={busy || (!text.trim() && !attachment)} className="flex items-center gap-2 rounded-xl bg-primary px-4 py-2.5 font-medium text-primary-foreground disabled:opacity-50">
                {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}收集并整理
              </button>
            </div>
          </form>
        </section>

        {error ? <div className="mb-5 rounded-xl bg-destructive/10 px-4 py-3 text-sm text-destructive">{error}</div> : null}
        {message ? <div className="mb-5 rounded-xl bg-emerald-500/10 px-4 py-3 text-sm text-emerald-700 dark:text-emerald-300">{message}</div> : null}

        <div className="grid gap-8 lg:grid-cols-[minmax(0,1.15fr)_minmax(320px,.85fr)]">
          <div className="space-y-8">
            <ItemSection title="已逾期" tone="text-destructive" items={grouped.overdue} onChanged={refresh} />
            <ItemSection title="今天" items={grouped.today} onChanged={refresh} />
            <ItemSection title="明天" items={grouped.tomorrow} onChanged={refresh} />
            <ItemSection title="尚未安排时间" items={grouped.unscheduled} onChanged={refresh} />
            <ItemSection title="之后" items={grouped.later} onChanged={refresh} />
            {!items.length ? <div className="rounded-2xl border border-dashed border-border p-10 text-center text-muted-foreground">还没有事项。说一句话或发一张截图开始吧。</div> : null}
          </div>
          <aside className="space-y-4">
            <div className="flex items-center gap-2"><Inbox className="h-5 w-5 text-primary" /><h2 className="font-semibold">待确认收件箱</h2><span className="rounded-full bg-muted px-2 py-0.5 text-xs">{pendingCaptures.length}</span></div>
            {pendingCaptures.map((capture) => <CaptureCard key={`${capture.id}-${capture.updatedAt}`} capture={capture} onChanged={refresh} />)}
            {!pendingCaptures.length ? <div className="rounded-2xl border border-dashed border-border p-6 text-center text-sm text-muted-foreground">AI 整理结果会先来到这里，由你确认后才进入日程。</div> : null}
          </aside>
        </div>
      </div>
    </main>
  )
}
