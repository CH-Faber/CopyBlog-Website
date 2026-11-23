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
                胡展榕，也叫 {profile.name}。来自广东茂名信宜，06 年的小孩，目前就读于
                <a href="https://www.jlu.edu.cn" target="_blank" rel="noopener noreferrer">
                  吉林大学
                </a>
                （JLU），生物技术专业，辅修软件工程。
              </p>

              <p>
                自我记事起，到我有自己的认知之前，我的生活一直就是不断去接受新的环境。小学一年级之前都一直在常州生活，之后就是漂泊不定的四处转学，甚至后来初高中入团的时候我还需要填六七行受教育经历。在常州待过，在惠州待过，也在贵州待过，最后还是回到了家乡信宜。并不是家庭生活丰富，而是因家庭经济情况四处奔波。
              </p>

              <p>
                我对于自我的认知，至少是在初高中那段个性被压得几乎没有余地的日子里，才慢慢产生的。大部分时候我连一个能躲的角落都没有——教室、宿舍、家，都是别人安排好的地方。但初三和高三那两年，对应的教学楼顶楼都有一间阁楼。很多时候我就静静待在那里，看着小说，又或者偷偷摸摸地玩玩手机。
              </p>

              <p>
                人越是被压抑，就越想做些什么。我开始不断去尝试体验，去感受：玩 Minecraft、学习 CS 写写代码、打打羽毛球、骑一下公路车、读一读知乎认知小文章、喝喝小茶、饮饮小咖，再到后来滑滑板、写写小文章小日记，跑跑步、跳跳绳、健健身、做做小网站、剪剪视频什么的。不过不少都只是探索，浅尝辄止，到最后我坚持下来的也就只有滑滑板，写些什么。
              </p>

              <p>
                二六上半年依旧母胎单身，却在大学的第三学期体验了两次分手。似乎我还是需要更多的成长，以及向内建设。
              </p>

              <p>
                本博客叫「一个闪念」。有很多时候，我们生活中会突然有一个乍现的想法闪过，但是如果不勤于记录，它的确就会一闪而过，故此取这个名字来勉励自己珍惜自己的每一个想法。
              </p>

              <p>
                虽然说目前我的闪念页很像朋友圈，但我会更希望接下来我能把它真正地利用起来，记录好自己的每一个想法。关于文章部分，我有心情就写，没心情的话基本 N 年不更新。但如果你看到了我目前的文字，并且让你有一点感触，或者说更加了解了我这个人，那这就是我的最大荣幸。
              </p>

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
