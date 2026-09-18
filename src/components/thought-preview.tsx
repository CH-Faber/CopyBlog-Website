import { profile } from "@/data/profile"

export type ThoughtPreviewItem = {
  slug: string
  title?: string
  date: string
  excerpt: string
  image?: string
}

function formatDate(dateStr: string) {
  const date = new Date(dateStr)
  if (Number.isNaN(date.getTime())) return dateStr
  return `${date.getMonth() + 1}月${date.getDate()}日 ${date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })}`
}

export function ThoughtPreview({
  thoughts,
  maxItems = 4,
}: {
  thoughts: ThoughtPreviewItem[]
  maxItems?: number
}) {
  const items = thoughts.slice(0, Math.max(0, maxItems))

  return (
    <div className="bg-card border border-border/50 rounded-xl p-5">
      <div className="flex items-center justify-between gap-3 mb-4">
        <div className="flex items-center gap-2">
          <img src={profile.avatar} alt={profile.name} width={28} height={28} className="h-7 w-7 rounded-full border border-border/70 object-cover" />
          <div>
            <h4 className="font-medium text-foreground">最近闪念</h4>
            <p className="text-[11px] text-muted-foreground">{profile.name} 的思想流</p>
          </div>
        </div>
        <a href="/thoughts/" className="text-xs text-muted-foreground hover:text-primary">全部</a>
      </div>

      {items.length > 0 ? (
        <div className="space-y-3">
          {items.map((thought) => (
            <a
              key={thought.slug}
              href={`/thoughts/#thought-${thought.slug}`}
              className="group flex gap-3 rounded-xl border-b border-border/50 px-1 py-3 last:border-b-0 transition-colors hover:bg-muted/60"
            >
              <img src={profile.avatar} alt="" width={32} height={32} className="mt-0.5 h-8 w-8 shrink-0 rounded-full object-cover" />
              <div className="min-w-0 flex-1">
                <div className="flex items-center justify-between gap-2 text-[11px] text-muted-foreground">
                  <span className="font-medium text-foreground">{profile.name}</span>
                  <span>{formatDate(thought.date)}</span>
                </div>
                {thought.title ? <div className="mt-1 line-clamp-1 text-xs font-medium text-foreground">{thought.title}</div> : null}
                <div className="mt-1 text-xs leading-relaxed text-muted-foreground line-clamp-3 whitespace-pre-line">{thought.excerpt}</div>
                {thought.image ? <img src={thought.image} alt="" loading="lazy" className="mt-2 h-16 w-16 rounded-lg object-cover" /> : null}
              </div>
            </a>
          ))}
        </div>
      ) : (
        <div className="text-sm text-muted-foreground">暂无闪念</div>
      )}

    </div>
  )
}
