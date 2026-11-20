import { Github, Mail, MessageCircle, Music, Globe } from "lucide-react"
import type { ProfileLinkType } from "@/data/profile"

export function ProfileIcon({ type, className }: { type: ProfileLinkType; className?: string }) {
  switch (type) {
    case "github":
      return <Github className={className} />
    case "email":
      return <Mail className={className} />
    case "qq":
      return <MessageCircle className={className} />
    case "music":
      return <Music className={className} />
    default:
      return <Globe className={className} />
  }
}
