export type ProfileLinkType = "qq" | "music" | "github" | "twitter" | "email"

export const profile: {
  name: string
  bio: string
  avatar: string
  links: { type: ProfileLinkType; name: string; url: string }[]
} = {
  name: "Faberl",
  bio: "在无数迷茫中寻找自己人生的定位",
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
