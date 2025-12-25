import { Header } from "@/components/header"
import { Footer } from "@/components/footer"
import { ExternalLink, Heart } from "lucide-react"

const friends = [
  {
    name: "数字花园",
    url: "https://example.com",
    avatar: "/digital-garden-logo-minimal.jpg",
    description: "一位专注于知识管理与个人成长的博主",
    tags: ["知识管理", "效率工具"],
  },
  {
    name: "代码与诗",
    url: "https://example.com",
    avatar: "/code-poetry-logo-minimal.jpg",
    description: "技术与人文的交汇处，探索编程的艺术",
    tags: ["编程", "人文"],
  },
  {
    name: "量化随笔",
    url: "https://example.com",
    avatar: "/quantitative-finance-logo-minimal.jpg",
    description: "从数据中寻找市场规律的量化交易者",
    tags: ["量化投资", "数据分析"],
  },
  {
    name: "AI 观察站",
    url: "https://example.com",
    avatar: "/ai-observatory-logo-minimal.jpg",
    description: "追踪 AI 前沿动态，解读技术变革",
    tags: ["人工智能", "深度学习"],
  },
  {
    name: "社会学笔记",
    url: "https://example.com",
    avatar: "/sociology-notes-logo-minimal.jpg",
    description: "用社会学视角观察日常生活的研究者",
    tags: ["社会学", "文化研究"],
  },
  {
    name: "无限游戏",
    url: "https://example.com",
    avatar: "/infinite-game-logo-minimal.jpg",
    description: "关于人生策略与长期主义的思考",
    tags: ["人生哲学", "长期主义"],
  },
]

export default function FriendsPage() {
  return (
    <div className="min-h-screen bg-background">
      <Header />
      <main className="pt-32 pb-20">
        <div className="max-w-4xl mx-auto px-6">
          {/* Header */}
          <section className="mb-12">
            <h1 className="text-4xl font-bold text-foreground mb-3 flex items-center gap-3">
              <span className="w-1 h-8 bg-primary rounded-full" />
              友情链接
            </h1>
            <p className="text-lg text-muted-foreground">这些是我欣赏的创作者们，他们在各自的领域持续输出优质内容。</p>
          </section>

          {/* Friends Grid */}
          <section className="mb-16">
            <div className="grid gap-4 sm:grid-cols-2">
              {friends.map((friend) => (
                <a
                  key={friend.name}
                  href={friend.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="group p-5 rounded-xl border border-border bg-card hover:border-primary/30 hover:shadow-lg transition-all duration-300"
                >
                  <div className="flex items-start gap-4">
                    {/* Avatar */}
                    <div className="shrink-0 w-14 h-14 rounded-xl overflow-hidden border border-border">
                      <img
                        src={friend.avatar || "/placeholder.svg"}
                        alt={friend.name}
                        width={56}
                        height={56}
                        loading="lazy"
                        className="object-cover w-full h-full"
                      />
                    </div>

                    {/* Info */}
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 mb-1">
                        <h3 className="font-semibold text-foreground truncate group-hover:text-primary transition-colors">
                          {friend.name}
                        </h3>
                        <ExternalLink className="w-3.5 h-3.5 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity" />
                      </div>
                      <p className="text-sm text-muted-foreground line-clamp-2 mb-2">{friend.description}</p>
                      <div className="flex flex-wrap gap-1.5">
                        {friend.tags.map((tag) => (
                          <span key={tag} className="px-2 py-0.5 text-xs rounded-md bg-secondary text-muted-foreground">
                            {tag}
                          </span>
                        ))}
                      </div>
                    </div>
                  </div>
                </a>
              ))}
            </div>
          </section>

          {/* Apply Section */}
          <section className="p-8 rounded-2xl border border-dashed border-border bg-secondary/30">
            <div className="text-center">
              <div className="inline-flex items-center justify-center w-12 h-12 rounded-full bg-primary/10 text-primary mb-4">
                <Heart className="w-6 h-6" />
              </div>
              <h2 className="text-xl font-semibold text-foreground mb-2">申请友链</h2>
              <p className="text-muted-foreground mb-6 max-w-md mx-auto">
                如果你也有一个持续更新的博客，欢迎互换友链。请确保你的博客内容原创、用心，并已添加本站链接。
              </p>
              <div className="inline-flex flex-col gap-3 text-left text-sm p-4 rounded-lg bg-background border border-border">
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground w-16">站点名称</span>
                  <span className="text-foreground font-medium">思维边界</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground w-16">站点地址</span>
                  <span className="text-foreground font-mono text-xs">https://example.com</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground w-16">站点描述</span>
                  <span className="text-foreground">金融、社会与 AI 的跨学科思考</span>
                </div>
              </div>
              <p className="text-sm text-muted-foreground mt-6">
                申请方式：发送邮件至 <span className="text-primary">hello@example.com</span>
              </p>
            </div>
          </section>
        </div>
      </main>
      <Footer />
    </div>
  )
}
