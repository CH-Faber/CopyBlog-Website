export type ProfileLinkType =
  | "github"
  | "email"
  | "qq"
  | "music"
  | "bilibili"
  | "douyin"
  | "wechat"
  | "linuxdo"
  | "x"

export const profile: {
  name: string
  bio: string
  avatar: string
  location: string
  mbti: string
  interests: string[]
  links: { type: ProfileLinkType; name: string; url: string }[]
} = {
  name: "Faber",
  bio: "在无数迷茫中寻找自己人生的定位",
  avatar: "/avatar.webp",
  location: "广东茂名",
  mbti: "INTP",
  interests: ["滑板", "Vibe Coding", "羽毛球", "健身", "听音乐", "看博客", "追番"],
  links: [
    {
      type: "github",
      name: "GitHub",
      url: "https://github.com/CH-Faber",
    },
    {
      type: "email",
      name: "邮箱",
      url: "mailto:hufaber120@gmail.com",
    },
  ],
}
