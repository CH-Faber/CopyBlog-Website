"use client"

import { useState } from "react"
import { Header } from "./header"
import { Footer } from "./footer"
import { BookOpen, Star, Calendar, Filter, Search, ExternalLink } from "lucide-react"
import { cn } from "@/lib/utils"

// 临时静态数据，之后可替换为飞书云文档数据源
const books = [
  {
    id: 1,
    title: "思考，快与慢",
    author: "丹尼尔·卡尼曼",
    cover: "/thinking-fast-and-slow-book-cover-minimal.jpg",
    rating: 5,
    status: "已读",
    category: "心理学",
    finishedDate: "2025-03",
    review: "系统1与系统2的双系统理论，深刻影响了我对决策的理解。",
  },
  {
    id: 2,
    title: "枪炮、病菌与钢铁",
    author: "贾雷德·戴蒙德",
    cover: "/guns-germs-and-steel-book-cover-minimal.jpg",
    rating: 5,
    status: "已读",
    category: "社会学",
    finishedDate: "2025-02",
    review: "从地理决定论视角解读人类文明发展的不平等，视野宏大。",
  },
  {
    id: 3,
    title: "债务：第一个5000年",
    author: "大卫·格雷伯",
    cover: "/debt-the-first-5000-years-book-cover-minimal.jpg",
    rating: 4,
    status: "已读",
    category: "金融",
    finishedDate: "2025-01",
    review: "颠覆了「物物交换-货币-信用」的传统叙事，人类学视角下的货币史。",
  },
  {
    id: 4,
    title: "规模",
    author: "杰弗里·韦斯特",
    cover: "/scale-geoffrey-west-book-cover-minimal.jpg",
    rating: 5,
    status: "已读",
    category: "复杂系统",
    finishedDate: "2024-12",
    review: "用幂律揭示生物、城市、公司的统一规律，跨学科思维的典范。",
  },
  {
    id: 5,
    title: "注意力商人",
    author: "吴修铭",
    cover: "/attention-merchants-book-cover-minimal.jpg",
    rating: 4,
    status: "已读",
    category: "社会学",
    finishedDate: "2024-11",
    review: "注意力如何成为商品，从报纸到社交媒体的注意力争夺史。",
  },
  {
    id: 6,
    title: "生成式人工智能",
    author: "埃隆·费雷拉",
    cover: "/generative-ai-book-cover-minimal.jpg",
    rating: 4,
    status: "在读",
    category: "AI",
    finishedDate: "",
    review: "系统梳理生成式AI的技术原理与应用场景。",
  },
  {
    id: 7,
    title: "随机漫步的傻瓜",
    author: "纳西姆·塔勒布",
    cover: "/fooled-by-randomness-book-cover-minimal.jpg",
    rating: 5,
    status: "已读",
    category: "金融",
    finishedDate: "2024-10",
    review: "运气与技能的区分，不确定性哲学的入门之作。",
  },
  {
    id: 8,
    title: "社会学的想象力",
    author: "C·赖特·米尔斯",
    cover: "/sociological-imagination-book-cover-minimal.jpg",
    rating: 4,
    status: "已读",
    category: "社会学",
    finishedDate: "2024-09",
    review: "将个人困扰与公共议题联结，社会学思维的经典表述。",
  },
]

const categories = ["全部", "金融", "社会学", "心理学", "AI", "复杂系统"]
const statuses = ["全部", "已读", "在读", "想读"]

export function BookshelfContent() {
  const [selectedCategory, setSelectedCategory] = useState("全部")
  const [selectedStatus, setSelectedStatus] = useState("全部")
  const [searchQuery, setSearchQuery] = useState("")

  const filteredBooks = books.filter((book) => {
    const matchCategory = selectedCategory === "全部" || book.category === selectedCategory
    const matchStatus = selectedStatus === "全部" || book.status === selectedStatus
    const matchSearch =
      book.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
      book.author.toLowerCase().includes(searchQuery.toLowerCase())
    return matchCategory && matchStatus && matchSearch
  })

  const stats = {
    total: books.length,
    finished: books.filter((b) => b.status === "已读").length,
    reading: books.filter((b) => b.status === "在读").length,
    avgRating: (books.reduce((sum, b) => sum + b.rating, 0) / books.length).toFixed(1),
  }

  return (
    <div className="min-h-screen bg-background">
      <Header />

      <main className="pt-32 pb-24">
        <div className="max-w-6xl mx-auto px-6">
          {/* Page Header */}
          <div className="mb-16">
            <div className="flex items-center gap-3 mb-4">
              <div className="w-1 h-8 bg-primary rounded-full" />
              <h1 className="text-4xl md:text-5xl font-bold text-foreground">书架</h1>
            </div>
            <p className="text-muted-foreground text-lg max-w-2xl">
              阅读是跨学科思维的基础。这里记录了我读过的书籍，涵盖金融、社会学、AI 等多个领域。
            </p>
          </div>

          {/* Stats */}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-12">
            <div className="bg-muted/30 rounded-2xl p-6 border border-border/50">
              <div className="flex items-center gap-3 mb-2">
                <BookOpen className="w-5 h-5 text-primary" />
                <span className="text-sm text-muted-foreground">总计</span>
              </div>
              <p className="text-3xl font-bold text-foreground">{stats.total}</p>
            </div>
            <div className="bg-muted/30 rounded-2xl p-6 border border-border/50">
              <div className="flex items-center gap-3 mb-2">
                <Calendar className="w-5 h-5 text-primary" />
                <span className="text-sm text-muted-foreground">已读</span>
              </div>
              <p className="text-3xl font-bold text-foreground">{stats.finished}</p>
            </div>
            <div className="bg-muted/30 rounded-2xl p-6 border border-border/50">
              <div className="flex items-center gap-3 mb-2">
                <BookOpen className="w-5 h-5 text-amber-500" />
                <span className="text-sm text-muted-foreground">在读</span>
              </div>
              <p className="text-3xl font-bold text-foreground">{stats.reading}</p>
            </div>
            <div className="bg-muted/30 rounded-2xl p-6 border border-border/50">
              <div className="flex items-center gap-3 mb-2">
                <Star className="w-5 h-5 text-primary" />
                <span className="text-sm text-muted-foreground">平均评分</span>
              </div>
              <p className="text-3xl font-bold text-foreground">{stats.avgRating}</p>
            </div>
          </div>

          {/* Filters */}
          <div className="flex flex-col md:flex-row gap-4 mb-8">
            {/* Search */}
            <div className="relative flex-1 max-w-md">
              <Search className="absolute left-4 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <input
                type="text"
                placeholder="搜索书名或作者..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full pl-11 pr-4 py-3 bg-muted/30 border border-border/50 rounded-xl text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary/50 transition-all"
              />
            </div>

            {/* Category Filter */}
            <div className="flex items-center gap-2 flex-wrap">
              <Filter className="w-4 h-4 text-muted-foreground" />
              {categories.map((category) => (
                <button
                  key={category}
                  onClick={() => setSelectedCategory(category)}
                  className={cn(
                    "px-4 py-2 rounded-full text-sm transition-all duration-200",
                    selectedCategory === category
                      ? "bg-primary text-primary-foreground"
                      : "bg-muted/50 text-muted-foreground hover:bg-muted hover:text-foreground",
                  )}
                >
                  {category}
                </button>
              ))}
            </div>
          </div>

          {/* Status Filter */}
          <div className="flex gap-2 mb-12">
            {statuses.map((status) => (
              <button
                key={status}
                onClick={() => setSelectedStatus(status)}
                className={cn(
                  "px-4 py-2 rounded-lg text-sm border transition-all duration-200",
                  selectedStatus === status
                    ? "border-primary text-primary bg-primary/5"
                    : "border-border/50 text-muted-foreground hover:border-border hover:text-foreground",
                )}
              >
                {status}
              </button>
            ))}
          </div>

          {/* Book Grid */}
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
            {filteredBooks.map((book) => (
              <div
                key={book.id}
                className="group bg-background border border-border/50 rounded-2xl overflow-hidden hover:border-border hover:shadow-lg transition-all duration-300"
              >
                {/* Cover */}
                <div className="relative aspect-[3/4] bg-muted/30 overflow-hidden">
                  <img
                    src={book.cover || "/placeholder.svg"}
                    alt={book.title}
                    className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500"
                  />
                  {/* Status Badge */}
                  <div
                    className={cn(
                      "absolute top-3 right-3 px-3 py-1 rounded-full text-xs font-medium",
                      book.status === "已读" && "bg-primary/90 text-primary-foreground",
                      book.status === "在读" && "bg-amber-500/90 text-white",
                      book.status === "想读" && "bg-muted/90 text-foreground",
                    )}
                  >
                    {book.status}
                  </div>
                </div>

                {/* Info */}
                <div className="p-5">
                  <h3 className="font-semibold text-foreground mb-1 line-clamp-1 group-hover:text-primary transition-colors">
                    {book.title}
                  </h3>
                  <p className="text-sm text-muted-foreground mb-3">{book.author}</p>

                  {/* Rating */}
                  <div className="flex items-center gap-1 mb-3">
                    {[...Array(5)].map((_, i) => (
                      <Star
                        key={i}
                        className={cn(
                          "w-4 h-4",
                          i < book.rating ? "fill-primary text-primary" : "text-muted-foreground/30",
                        )}
                      />
                    ))}
                  </div>

                  {/* Review */}
                  <p className="text-sm text-muted-foreground line-clamp-2">{book.review}</p>

                  {/* Meta */}
                  <div className="flex items-center justify-between mt-4 pt-4 border-t border-border/50">
                    <span className="text-xs text-muted-foreground bg-muted/50 px-2 py-1 rounded">{book.category}</span>
                    {book.finishedDate && <span className="text-xs text-muted-foreground">{book.finishedDate}</span>}
                  </div>
                </div>
              </div>
            ))}
          </div>

          {filteredBooks.length === 0 && (
            <div className="text-center py-16">
              <BookOpen className="w-12 h-12 text-muted-foreground/30 mx-auto mb-4" />
              <p className="text-muted-foreground">没有找到匹配的书籍</p>
            </div>
          )}

          {/* Data Source Notice */}
          <div className="mt-16 p-6 bg-muted/20 rounded-2xl border border-border/50">
            <div className="flex items-start gap-4">
              <div className="w-10 h-10 rounded-xl bg-primary/10 flex items-center justify-center flex-shrink-0">
                <ExternalLink className="w-5 h-5 text-primary" />
              </div>
              <div>
                <h4 className="font-medium text-foreground mb-1">数据源</h4>
                <p className="text-sm text-muted-foreground">
                  本书架数据来源于飞书云文档，持续更新中。如需获取完整书单或阅读笔记，欢迎
                  <a href="#" className="text-primary hover:underline ml-1">
                    联系我
                  </a>
                  。
                </p>
              </div>
            </div>
          </div>
        </div>
      </main>

      <Footer />
    </div>
  )
}
