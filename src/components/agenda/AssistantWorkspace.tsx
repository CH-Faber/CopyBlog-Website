import { useCallback, useEffect, useState } from "react"
import { ArrowLeft, Loader2, LogOut, Sparkles } from "lucide-react"
import { organizerApi, OrganizerApiError } from "@/lib/organizer-api"
import { AgendaLogin } from "./AgendaApp"
import { AssistantPanel } from "./AssistantPanel"

function messageFor(error: unknown) {
  if (error instanceof OrganizerApiError) return error.message
  if (error instanceof Error) return error.message
  return "无法连接 Agenda 服务"
}

export function AssistantWorkspace() {
  const [authenticated, setAuthenticated] = useState<boolean | null>(null)
  const [error, setError] = useState("")

  const checkSession = useCallback(async () => {
    try {
      await organizerApi.session()
      setAuthenticated(true)
      setError("")
    } catch (caught) {
      if (caught instanceof OrganizerApiError && caught.status === 401) setAuthenticated(false)
      else setError(messageFor(caught))
    }
  }, [])

  useEffect(() => { void checkSession() }, [checkSession])

  if (authenticated === null) return <main className="flex min-h-screen items-center justify-center bg-background"><Loader2 className="h-7 w-7 animate-spin text-primary" /></main>
  if (!authenticated) return <AgendaLogin onAuthenticated={() => void checkSession()} />

  async function logout() {
    try { await organizerApi.logout(); setAuthenticated(false) }
    catch (caught) { setError(messageFor(caught)) }
  }

  return <main className="agenda-root min-h-screen bg-background text-foreground">
    <div className="agenda-assistant-shell min-h-screen md:grid md:grid-cols-[248px_minmax(0,1fr)]">
      <aside className="agenda-assistant-sidebar hidden border-r border-border bg-card md:flex md:flex-col">
        <div className="border-b border-border px-5 py-5"><a href="/agenda/" className="mb-5 inline-flex items-center gap-2 text-xs text-muted-foreground hover:text-foreground"><ArrowLeft className="h-3.5 w-3.5" />返回 Agenda</a><div className="flex items-center gap-2"><span className="bg-primary/10 p-2 text-primary"><Sparkles className="h-4 w-4" /></span><div><p className="text-xs text-muted-foreground">Agenda</p><h1 className="font-semibold">AI 工作台</h1></div></div></div>
        <div className="flex-1 px-4 py-5"><p className="text-xs font-semibold uppercase tracking-[0.14em] text-muted-foreground">你可以直接说</p><div className="mt-3 space-y-2 text-sm text-muted-foreground"><p className="border-l-2 border-primary/40 px-3 py-2">安排下周的课程和健身</p><p className="border-l-2 border-primary/40 px-3 py-2">找出一直没有推进的任务</p><p className="border-l-2 border-primary/40 px-3 py-2">把周三的会议改到周四</p></div></div>
        <div className="border-t border-border p-4"><button onClick={() => void logout()} className="agenda-nav-item flex w-full items-center gap-2 px-3 py-2 text-sm text-muted-foreground hover:text-foreground"><LogOut className="h-4 w-4" />退出登录</button></div>
      </aside>
      <section className="min-w-0">
        <header className="agenda-topbar flex h-16 items-center justify-between border-b border-border px-4 sm:px-6"><div className="flex items-center gap-3"><a href="/agenda/" className="p-2 text-muted-foreground hover:bg-muted md:hidden" aria-label="返回 Agenda"><ArrowLeft className="h-5 w-5" /></a><div><p className="text-xs text-muted-foreground">Agenda AI</p><h2 className="font-semibold">把想法变成可执行的安排</h2></div></div><button onClick={() => void logout()} className="agenda-icon-button p-2.5 text-muted-foreground md:hidden" aria-label="退出登录"><LogOut className="h-4 w-4" /></button></header>
        {error ? <div className="mx-auto mt-4 max-w-5xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">{error}</div> : null}
        <div className="mx-auto max-w-5xl px-3 py-4 sm:px-6 sm:py-6"><AssistantPanel embedded onChanged={async () => {}} /></div>
      </section>
    </div>
  </main>
}
