import { Github, MapPin } from "lucide-react"
import { profile } from "@/data/profile"
import { ProfileIcon } from "@/components/profile-icon"

const SUPPORT_URL = "https://github.com/CH-Faber"

export default function AboutPage({
  heading = "关于我",
  subtitle = profile.bio,
}: {
  heading?: string
  subtitle?: string
}) {
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
                  {heading}
                </h1>
                <p className="text-xl text-muted-foreground mb-4">{subtitle}</p>

                <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-muted-foreground mb-6">
                  <span className="flex items-center gap-1.5">
                    <MapPin className="w-4 h-4" />
                    {profile.location}
                  </span>
                  <span aria-hidden="true" className="text-muted-foreground/40">
                    ·
                  </span>
                  <span>MBTI {profile.mbti}</span>
                </div>

                {/* Social Links */}
                <div className="flex flex-wrap items-center gap-2">
                  {profile.links.map((link) => (
                    <a
                      key={link.name}
                      href={link.url}
                      target={link.type === "email" ? undefined : "_blank"}
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-2 rounded-lg bg-secondary px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-secondary/80 hover:text-foreground"
                    >
                      <ProfileIcon type={link.type} className="w-4 h-4" />
                      <span>{link.name}</span>
                    </a>
                  ))}
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
                不管发生什么，我再也不愿品尝那种痛楚了。与其遭受那种苦境，还不如孤独一人静静地离群索居。
                <cite className="mt-3 block text-sm not-italic text-muted-foreground">— 村上春树</cite>
              </blockquote>
              <p>
                你好，我是胡展榕（{profile.name}），欢迎来到我的博客。
              </p>
              <p>
                现在是一名就读于
                <a href="https://www.jlu.edu.cn" target="_blank" rel="noopener noreferrer">
                  JLU
                </a>
                的 25 届本科生，来自广东茂名信宜。
              </p>

              <div className="not-prose my-7">
                <p className="mb-3 text-sm text-muted-foreground">平时在这些事情上花时间：</p>
                <ul className="flex list-none flex-wrap gap-2 pl-0">
                  {profile.interests.map((interest) => (
                    <li
                      key={interest}
                      className="rounded-full border border-border bg-secondary/60 px-3.5 py-1.5 text-sm text-foreground/80"
                    >
                      {interest}
                    </li>
                  ))}
                </ul>
              </div>

              <p>
                📬 联系我：<a href="mailto:hufaber120@gmail.com">hufaber120@gmail.com</a>
              </p>
            </div>
          </section>

          {/* Support Section */}
          <section className="mb-16">
            <h2 className="text-2xl font-semibold text-foreground mb-6 flex items-center gap-3">
              <span className="w-1 h-6 bg-primary rounded-full" />
              支持我
            </h2>
            <div className="rounded-2xl border border-border bg-card p-6">
              <p className="text-muted-foreground text-sm leading-relaxed">
                这里没有收款码。如果这些内容对你有帮助，欢迎来 GitHub 找我聊两句，或者给这个博客的开源项目点一个 Star —— 那对我来说就是最好的支持。
              </p>
              <a
                href={SUPPORT_URL}
                target="_blank"
                rel="noopener noreferrer"
                className="mt-5 inline-flex items-center gap-2 rounded-lg bg-foreground px-4 py-2.5 text-sm font-medium text-background transition-opacity hover:opacity-90"
              >
                <Github className="w-4 h-4" />
                去 GitHub 看看
              </a>
            </div>
          </section>

        </div>
      </main>
    </div>
  )
}
