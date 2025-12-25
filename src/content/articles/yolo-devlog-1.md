---
title: "YOLO 开发日志（一）：为什么要开发 YOLO"
excerpt: "为什么从零开始做 YOLO，以及对 Smart Composer 的改造思路。"
category: "ai"
categoryLabel: "深度学习"
tag: "YOLO开发日志"
date: "2025-10-05"
wordCount: 1285
readTime: "6 分钟"
---
## 起因

主要还是因为我对 Smart Composer 的不满，所以自己动手 fork 了一份，照着自己的理念彻底魔改一番。

### Smart Composer 的问题

Smart Composer 是一个优秀的工具，但它有几个让我难以忍受的问题：

1. **响应速度**：在处理大型项目时，响应明显变慢
2. **上下文管理**：对长对话的上下文处理不够智能
3. **自定义能力**：很多行为是硬编码的，无法根据个人习惯调整

### YOLO 的设计理念

YOLO 的核心理念是：**少即是多，小步快跑**。

```typescript
// 简单的架构，清晰的边界
interface YOLOCore {
  context: ContextManager
  executor: CommandExecutor  
  renderer: UIRenderer
}
```

我不想做一个功能大而全的工具，而是想做一个：

- **快**：响应要快，操作要快
- **准**：理解意图要准，执行结果要准
- **简**：界面简洁，概念简单

### 技术选型

| 模块 | 选择 | 理由 |
|------|------|------|
| 语言 | TypeScript | 类型安全，生态丰富 |
| 框架 | Next.js | SSR 支持，路由简单 |
| AI | Claude API | 长上下文，代码能力强 |

---

下一篇将介绍 YOLO 的具体架构设计。
