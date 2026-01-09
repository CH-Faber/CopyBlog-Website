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
                    中国
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
                我也明知白驹过隙，逝者如斯。未来可能讲更有意思的话，著更其完美的文，做更其壮丽的事业，但今天只是今天，未来也只是今天的未来。
              </blockquote>
              <p>
                你好，我是时歌，或许你也可以叫我 Lapis0x0。
              </p>
              <p>
                2004年出生在中国，我做过老师，当过私募的分析师，也写过一点点程序，读过一些书，跑过一点点远路。对世界充满兴趣，也总是心怀戒备。
              </p>
              <p>我对很多事感兴趣——</p>
              <ul>
                <li>金融学与法学，理性世界的两翼；</li>
                <li>机器学习，尤其是大语言模型的演化逻辑；</li>
                <li>也关注心理学、社会学、传媒工程；</li>
                <li>闲暇时沉迷于二游文化、开源情报、游戏编剧、以及偶像P活。</li>
              </ul>
              <p>
                <strong>用作社交标签的 MBTI：INTJ-A（架构师）</strong>
              </p>
              <p>
                📬 联系邮箱：<a href="mailto:lapiscafe@foxmail.com">lapiscafe@foxmail.com</a>
              </p>
            </div>
          </section>

          {/* Appreciation Section */}
          <section className="mb-16">
            <h2 className="text-2xl font-semibold text-foreground mb-6 flex items-center gap-3">
              <span className="w-1 h-6 bg-primary rounded-full" />
              赞赏支持
            </h2>
            <div className="p-6 rounded-2xl border border-border bg-card">
              <div className="flex items-start gap-3 mb-6">
                <Heart className="w-5 h-5 text-primary mt-0.5" />
                <p className="text-muted-foreground text-sm leading-relaxed">
                  如果你觉得我的博客内容对你有所帮助或启发，可以考虑赞赏支持我继续创作。你的每一份支持都是我前进的动力，非常感谢！
                </p>
              </div>
              <div className="flex justify-center gap-8 flex-wrap">
                <div className="text-center">
                  <div className="w-52 h-52 rounded-xl overflow-hidden border border-border mb-2">
                    <img src="/images/vote/weixin.jpg" alt="微信赞赏码" className="w-full h-full object-cover" />
                  </div>
                  <p className="text-sm font-medium text-green-600">微信赞赏</p>
                </div>
                <div className="text-center">
                  <div className="w-52 h-52 rounded-xl overflow-hidden border border-border mb-2">
                    <img src="/images/vote/zhifubao.jpg" alt="支付宝赞赏码" className="w-full h-full object-cover" />
                  </div>
                  <p className="text-sm font-medium text-blue-600">支付宝赞赏</p>
                </div>
              </div>
            </div>
          </section>

        </div>
      </main>
    </div>
  )
}
