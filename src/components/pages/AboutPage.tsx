import { MapPin, Github, MessageCircle, Music, Heart } from "lucide-react"
import { profile } from "@/data/profile"

export default function AboutPage() {
  return (
    <div className="min-h-screen bg-background dark:bg-muted/30">
      <main className="pt-32 pb-8">
        <div className="max-w-4xl mx-auto px-6">
          {/* Hero Section */}
          <section className="mb-10">
            <div className="flex flex-col md:flex-row gap-10 items-start">
              {/* Avatar */}
              <div className="shrink-0">
                <div className="w-36 h-36 rounded-2xl overflow-hidden border-2 border-border shadow-lg">
                  <img
                    src={profile.avatar}
                    alt={profile.name}
                    width={144}
                    height={144}
                    loading="lazy"
                    className="object-cover w-full h-full"
                  />
                </div>
              </div>

              {/* Info */}
              <div className="flex-1">
                <h1 className="text-4xl font-bold text-foreground mb-2 flex items-center gap-3">
                  <span className="w-1 h-8 bg-primary rounded-full" />
                  关于我
                </h1>
                <p className="text-xl text-muted-foreground mb-4">{profile.bio}</p>

                <div className="flex flex-wrap items-center gap-4 text-sm text-muted-foreground mb-6">
                  <span className="flex items-center gap-1.5">
                    <MapPin className="w-4 h-4" />
                    地球
                  </span>
                </div>

                {/* Social Links */}
                <div className="flex items-center gap-3">
                  {profile.links.map((link) => {
                    const Icon = link.type === "qq" ? MessageCircle : link.type === "music" ? Music : Github
                    return (
                      <a
                        key={link.name}
                        href={link.url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="p-2.5 rounded-lg bg-secondary hover:bg-secondary/80 transition-colors"
                        aria-label={link.name}
                      >
                        <Icon className="w-5 h-5 text-muted-foreground" />
                      </a>
                    )
                  })}
                </div>
              </div>
            </div>
          </section>

          {/* Bio Section */}
          <section className="mb-16">
            <div className="prose prose-neutral dark:prose-invert max-w-none
            prose-p:text-base prose-p:leading-8 prose-p:my-5 prose-p:text-foreground/90
            prose-a:text-primary prose-a:no-underline prose-a:hover:underline
            prose-blockquote:border-l-primary prose-blockquote:border-l-2 prose-blockquote:pl-4 prose-blockquote:italic prose-blockquote:text-foreground/70 prose-blockquote:font-normal prose-blockquote:bg-muted/20 prose-blockquote:py-2 prose-blockquote:rounded-r-lg
            prose-strong:text-foreground prose-strong:font-semibold
            prose-ul:text-foreground/90 prose-ul:my-5
            prose-li:marker:text-primary prose-li:my-1.5"
            >
              <blockquote>
                "这里可以放一段你喜欢的名言。"
              </blockquote>
              <p>
                你好，我是{profile.name}，欢迎来到我的博客。
              </p>
              <p>
                这里是个人简介占位。你可以介绍自己的背景、职业和兴趣爱好。
              </p>
              <p>我的兴趣方向：</p>
              <ul>
                <li>Web 开发与设计</li>
                <li>写作与知识分享</li>
                <li>探索新技术</li>
              </ul>
              <p>
                <strong>MBTI：INTP</strong>
              </p>
              <p>
                📬 联系我：<a href="mailto:hello@example.com">hello@example.com</a>
              </p>
            </div>
          </section>

          {/* Appreciation Section */}
          <section className="mb-16">
            <h2 className="text-2xl font-semibold text-foreground mb-6 flex items-center gap-3">
              <span className="w-1 h-6 bg-primary rounded-full" />
              支持我
            </h2>
            <div className="p-6 rounded-2xl border border-border bg-card">
              <div className="flex items-start gap-3 mb-6">
                <Heart className="w-5 h-5 text-primary mt-0.5" />
                <p className="text-muted-foreground text-sm leading-relaxed">
                  如果你觉得我的内容有帮助，欢迎支持我！（此处可放置捐赠链接或二维码）
                </p>
              </div>
            </div>
          </section>

        </div>
      </main>
    </div>
  )
}
