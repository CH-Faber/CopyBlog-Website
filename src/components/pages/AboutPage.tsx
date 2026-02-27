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
                    Earth
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
                "This is a sample blockquote. You can put your favorite quote here."
              </blockquote>
              <p>
                Hi, I'm {profile.name}. Welcome to my blog.
              </p>
              <p>
                This is a placeholder for your biography. You can describe your background, your profession, and what you love to do.
              </p>
              <p>I am interested in:</p>
              <ul>
                <li>Web Development and Design</li>
                <li>Writing and sharing knowledge</li>
                <li>Exploring new technologies</li>
              </ul>
              <p>
                <strong>MBTI: INTP</strong>
              </p>
              <p>
                📬 Contact: <a href="mailto:hello@example.com">hello@example.com</a>
              </p>
            </div>
          </section>

          {/* Appreciation Section */}
          <section className="mb-16">
            <h2 className="text-2xl font-semibold text-foreground mb-6 flex items-center gap-3">
              <span className="w-1 h-6 bg-primary rounded-full" />
              Support Me
            </h2>
            <div className="p-6 rounded-2xl border border-border bg-card">
              <div className="flex items-start gap-3 mb-6">
                <Heart className="w-5 h-5 text-primary mt-0.5" />
                <p className="text-muted-foreground text-sm leading-relaxed">
                  If you find my content helpful, consider supporting me! (Placeholder for your donation links/QR codes)
                </p>
              </div>
            </div>
          </section>

        </div>
      </main>
    </div>
  )
}
