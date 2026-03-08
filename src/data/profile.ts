export type ProfileLinkType = "qq" | "music" | "github" | "twitter" | "email"

export const profile: {
  name: string
  bio: string
  avatar: string
  links: { type: ProfileLinkType; name: string; url: string }[]
} = {
  name: "Faber",
  bio: "探索金融、社会与人工智能的交汇点",
  avatar: "/avatar.webp",
  links: [
    {
      type: "github",
      name: "GitHub",
      url: "https://github.com/yourusername",
    },
    {
      type: "twitter",
      name: "Twitter",
      url: "https://twitter.com/yourusername",
    },
    {
      type: "email",
      name: "邮箱",
      url: "mailto:hello@example.com",
    },
  ],
}
