# Agenda 独立平台化重构设计

**状态：** 已获用户确认，待实施计划复核

**日期：** 2026-09-22

## 1. 目标

将当前嵌入个人网站的 Agenda 重构为独立的个人日程平台：Agenda API 成为私人数据的唯一拥有者，Agenda Web 和 Android 是完整客户端，个人网站只读取经过用户选择和脱敏的公开日程投影。第一阶段保留现有单用户体验和 SQLite 数据，预留 workspace、多用户、自托管和外部日历扩展能力。

## 2. 非目标

第一阶段不实现以下内容：

- Google Calendar、Outlook 或 CalDAV 的双向同步；
- 完整注册、邀请、团队协作和计费系统；
- 用户级 AI Key 管理；
- 微服务集群、Kubernetes 或 PostgreSQL 必选化；
- 复杂重复事项编辑器、拖拽日历和公开日程评论。

第一阶段只实现服务边界、公开投影、共享客户端契约和迁移保护网。

## 3. 核心原则

1. Agenda API 拥有全部私人数据；任何客户端不得直接访问 Agenda 数据库。
2. 个人网站只能访问公开投影，不能通过过滤私人 `items` 表来生成公开响应。
3. 新事项默认私有；公开必须由用户显式发布，并可随时撤销。
4. Capture 原文不可被 AI 结果覆盖；AI 只能产生候选建议。
5. AI 候选、正式 Agenda Item、外部日历事件和网站公开条目是四种不同对象。
6. 第一阶段保持旧 API 和 `/agenda/` 路径兼容，采用增量迁移而非一次性重写。
7. 所有核心数据从第一阶段开始具备 `workspace_id` 归属，即使暂时只有一个个人 workspace。
8. Web、Android 和未来客户端使用统一的 API 契约和客户端层；平台能力通过 adapter 注入。

## 4. 目标系统边界

### 4.1 组件

```text
VermilionVoid/
├── apps/
│   ├── agenda-web/              # 独立私人 Agenda Web 客户端
│   └── agenda-android/          # Capacitor Android 客户端
├── services/
│   └── agenda-api/              # 独立 Go API 服务
├── packages/
│   ├── agenda-contracts/        # 纯 TypeScript 契约与状态
│   ├── agenda-client/            # Web/Android 共用 API 客户端
│   ├── agenda-ui/                # 私人 Agenda UI
│   └── agenda-platform/          # Web/Android 平台能力接口与实现
├── src/
│   └── components/public-agenda/ # 网站公开只读组件
└── docs/agenda/                  # 部署、API 和自托管文档
```

迁移期间可以保留当前物理目录，但逻辑边界必须先成立；`scripts/organizer` 逐步迁移为 `services/agenda-api`，`src/components/agenda` 逐步迁移为 `packages/agenda-ui`。

### 4.2 消费者权限

| 消费者 | 能力 |
| --- | --- |
| Agenda Web | 完整私人读写、发布设置、连接管理 |
| Agenda Android | 完整私人读写、设备提醒、设备令牌 |
| 个人网站 | 只读公开 Agenda API |
| 未认证访客 | 只能读取明确发布且未过期的公开条目 |

### 4.3 过渡期 URL

```text
/api/agenda/v1/*       新私人 API
/api/agenda/public/v1/* 公开 API
/api/organizer/v1/*    旧客户端兼容入口
/agenda/               旧书签兼容入口
```

生产环境第一阶段仍由 Nginx 将 `/api/agenda/` 反代到独立 API 进程，暂不强制更换域名或引入跨域 Cookie。未来可以迁移为 `agenda.faberhu.top` 与 `api.agenda.faberhu.top`。

## 5. 身份和 Workspace

### 5.1 数据模型

引入：

```text
users
├── id
├── email
├── display_name
├── password_hash
├── status
├── created_at
└── updated_at

workspaces
├── id
├── owner_user_id
├── slug
├── name
├── timezone
├── status
├── created_at
└── updated_at

workspace_members
├── workspace_id
├── user_id
├── role
├── created_at
└── updated_at
```

第一阶段启动或迁移时创建一个默认个人用户和个人 workspace。当前所有业务数据归属于该 workspace。所有核心表新增非空 `workspace_id`，默认填入该 workspace。

### 5.2 Principal

认证中间件统一返回：

```go
type Principal struct {
    UserID      string
    WorkspaceID string
    DeviceID    string
    AuthMethod  string
}
```

业务 handler 不再直接判断 Cookie、Bearer 或 Android，而是依赖已经验证过的 Principal。现有浏览器 Session 和 Android Device Token 在迁移期继续支持，并关联到 user/workspace。

## 6. 私人数据、公开投影和外部事件

### 6.1 Capture 与 AI

Capture 保存不可变原始输入：

```text
received → parsing → needs_review → confirmed
                       └──────────→ failed
```

每次 AI 解析创建独立 `capture_runs`：

```text
capture_runs
├── id
├── capture_id
├── provider
├── model
├── status
├── request_hash
├── result_json
├── error_code
├── error_message
├── started_at
├── completed_at
└── created_at
```

AI 候选不会直接写入正式 `items`。用户确认时才解析 `projectName → projectId`、校验时间、归一化时区并创建 Item。确认接口使用幂等键，避免网络重试重复生成事项。

### 6.2 正式 Item

正式 Item 是 Agenda 内部事实来源，包含 `workspace_id`、`version` 和 `project_id`。更新使用 expected version；版本不匹配返回 409，客户端必须重新读取而不是覆盖。

### 6.3 公开发布

新增 `agenda_publications` 和 `agenda_public_entries`。新事项默认没有 publication。公开级别：

- `private`：不公开；
- `public`：出现在公开列表；
- `unlisted`：只有分享链接可见；
- `expired`：曾经发布但已经过期。

展示级别：

- `full`：公开允许字段；
- `summary`：公开标题和摘要；
- `busy`：只公开时间段。

网站只查询 `agenda_public_entries`，不查询私人 `items`。

### 6.4 外部日历

外部事件保存在 `external_events`，不伪装成 Agenda Item。第一阶段只做 ICS 导入、ICS 导出和只读连接器。Google、Outlook、CalDAV 双向同步必须等外部事件 ID、版本/ETag、冲突和删除规则稳定后再实现。

## 7. 客户端结构

### 7.1 共享层

`agenda-contracts` 只包含纯类型、状态、验证规则和公开模型；不依赖 React、Astro 或 Capacitor。

`agenda-client` 提供统一接口：

```ts
interface AgendaClient {
  session(): Promise<Session>
  login(input: LoginInput): Promise<AuthSession>
  logout(): Promise<void>
  listItems(query?: ItemQuery): Promise<ItemPage>
  createItem(input: CreateItemInput): Promise<Item>
  updateItem(input: UpdateItemInput): Promise<Item>
  completeItem(id: string, version: number): Promise<Item>
  createCapture(input: CreateCaptureInput): Promise<Capture>
  parseCapture(id: string): Promise<CaptureRun>
  confirmCapture(input: ConfirmCaptureInput): Promise<ConfirmResult>
  updatePublication(input: UpdatePublicationInput): Promise<AgendaPublication>
}
```

### 7.2 平台 adapter

```ts
interface AgendaPlatform {
  auth: AuthProvider
  notifications: NotificationPlatform
  storage: StoragePlatform
  lifecycle: LifecyclePlatform
  openItem(itemId: string): void
}
```

Web 提供 Cookie Session、Service Worker、Web Push 和浏览器存储；Android 提供 Secure Storage、Bearer Token、本机通知、Exact Alarm、前后台生命周期和通知动作。

私人 UI 不再使用 `isNativeAgenda()` 分支处理平台差异。

### 7.3 个人网站

个人网站只导入 `PublicAgendaEntry` 和公开客户端，提供公开时间轴、日历和 ICS 入口，不加载登录、Capture、AI、私人项目或设备设置。

## 8. 部署和开源

第一阶段继续使用 SQLite 和单机目录：

```text
/var/lib/agenda/
├── agenda.db
├── attachments/
├── backups/
├── exports/
└── logs/
```

新的配置使用 `AGENDA_*` 前缀，旧 `ORGANIZER_*` 在迁移期兼容。API 提供 `/healthz`、`/readyz` 和 `/version`。日志不得包含密码、token、AI Key、OAuth token 或 Capture 完整正文。

开源版本提供 Dockerfile、docker-compose.yml、`.env.example`、数据库 migrations、初始化命令、数据导入导出和自托管文档。PostgreSQL 作为未来可选后端，不阻塞 SQLite 单机部署。

## 9. 迁移阶段

1. **Phase 0：保护网** —— 状态/API/公开契约、数据库快照、集成和 E2E 回归测试。
2. **Phase 1：服务边界** —— `services/agenda-api`、`AGENDA_*` 配置、新路径、旧路径兼容、默认 workspace。
3. **Phase 2：公开投影** —— publication、public entry、公开 API、个人网站公开日程和 ICS。
4. **Phase 3：客户端抽离** —— contracts、client、UI、Agenda Web 独立构建、Android 共享客户端。
5. **Phase 4：Android 收口** —— 平台 adapter、通知增量同步、通知打开事项、前后台和离线提示。
6. **Phase 5：外部日历只读同步** —— ICS 导入/导出、external events、连接状态和重试。
7. **Phase 6：开源多用户基础** —— 用户、workspace 成员、初始化命令、Docker Compose、API token 和权限。
8. **Phase 7：高级同步** —— Google/Outlook/CalDAV、双向同步、冲突处理和用户级 AI 配置。

每个阶段必须保留可运行的 Web、Android 和 API 版本；不采用一次性重写。

## 10. 验收标准

- Agenda API 可以脱离 Astro 网站独立启动；
- Agenda Web 和 Android 使用同一套业务契约；
- 个人网站只读取公开投影；
- 私人事项默认不公开，公开后可以撤销；
- Capture 原文不会因 AI 失败丢失；
- AI 重试不会覆盖历史运行记录；
- 确认操作不会因网络重试重复创建 Item；
- Item 版本冲突不会静默覆盖；
- 现有 SQLite 数据完整迁移到默认 workspace；
- `/api/organizer/v1` 和 `/agenda/` 在迁移期继续可用；
- 新实例可以使用 SQLite 自托管；
- 公开 ICS 可以由其他日历订阅；
- 迁移后的安全默认值是“私有优先”。
