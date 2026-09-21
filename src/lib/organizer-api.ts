export type Candidate = {
  type: "task" | "event" | "reminder" | "note"
  title: string
  description?: string
  startAt?: string
  endAt?: string
  dueAt?: string
  reminderAt?: string
  timezone?: string
  allDay?: boolean
  recurrenceRule?: string
  priority?: number
  certainty?: "confirmed" | "tentative"
  durationMinutes?: number
  availableFrom?: string
  availableUntil?: string
  projectId?: string
  project?: string
  tags?: string[]
  location?: string
  people?: string[]
  confidence?: number
  ambiguities?: string[]
}

export type Capture = {
  id: string
  sourceType: string
  rawText: string
  attachmentName?: string
  attachmentMime?: string
  hasAttachment: boolean
  status: "received" | "parsing" | "needs_review" | "confirmed" | "failed"
  aiResult?: { items: Candidate[] }
  error?: string
  createdAt: string
  updatedAt: string
}

export type OrganizerItem = Candidate & {
  id: string
  captureId?: string
  timezone: string
  status: "inbox" | "todo" | "doing" | "done" | "cancelled" | "archived"
  certainty: "confirmed" | "tentative"
  version: number
  completedAt?: string
  cancelledAt?: string
  archivedAt?: string
  createdAt: string
  updatedAt: string
}

export type Project = {
  id: string
  name: string
  description?: string
  status: "active" | "waiting" | "blocked" | "completed" | "archived"
  priority: number
  color?: string
  goal?: string
  deadline?: string
  reviewAt?: string
  itemCount: number
  openCount: number
  doneCount: number
  createdAt: string
  updatedAt: string
}

export type ItemEvent = {
  id: string
  itemId: string
  eventType: "created" | "updated" | "completed" | "cancelled" | "reopened" | "archived" | "snoozed" | "status_changed"
  data?: unknown
  createdAt: string
}

export type Memory = {
  id: string
  kind: "preference" | "procedure" | "example"
  key?: string
  content: string
  scope: string
  status: "proposed" | "active" | "dismissed" | "forgotten"
  evidenceCount: number
  createdAt: string
  updatedAt: string
}

export class OrganizerApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}

const base = "/api/organizer/v1"

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body && !(init.body instanceof FormData)) headers.set("Content-Type", "application/json")
  const response = await fetch(`${base}${path}`, {
    ...init,
    headers,
    credentials: "same-origin",
  })
  const text = await response.text()
  let body: unknown = null
  if (text) {
    try {
      body = JSON.parse(text)
    } catch {
      body = text
    }
  }
  if (!response.ok) {
    const message = typeof body === "object" && body && "message" in body ? String(body.message) : text || `HTTP ${response.status}`
    throw new OrganizerApiError(response.status, message)
  }
  return body as T
}

export const organizerApi = {
  login: (password: string) => request<{ authenticated: boolean }>("/auth/login", { method: "POST", body: JSON.stringify({ password }) }),
  logout: () => request("/auth/logout", { method: "POST", body: "{}" }),
  session: () => request<{ authenticated: boolean; pushEnabled: boolean; timezone: string }>("/session"),
  changePassword: (currentPassword: string, newPassword: string) =>
    request("/password", { method: "PUT", body: JSON.stringify({ currentPassword, newPassword }) }),
  createDeviceToken: (name: string) =>
    request<{ id: string; token: string; message: string }>("/device-tokens", { method: "POST", body: JSON.stringify({ name }) }),
  listCaptures: () => request<{ captures: Capture[] }>("/captures?limit=100"),
  createCapture: (rawText: string, attachment?: File) => {
    if (attachment) {
      const form = new FormData()
      form.set("rawText", rawText)
      form.set("sourceType", "image")
      form.set("attachment", attachment)
      return request<Capture>("/captures", { method: "POST", body: form })
    }
    return request<Capture>("/captures", { method: "POST", body: JSON.stringify({ rawText, sourceType: "voice_text" }) })
  },
  parseCapture: (id: string) => request<{ items: Candidate[] }>(`/captures/${encodeURIComponent(id)}/parse`, { method: "POST", body: "{}" }),
  deleteCapture: (id: string) => request<void>(`/captures/${encodeURIComponent(id)}`, { method: "DELETE" }),
  confirmCapture: (id: string, items: Candidate[]) =>
    request<{ items: OrganizerItem[] }>(`/captures/${encodeURIComponent(id)}/confirm`, { method: "POST", body: JSON.stringify({ items }) }),
  listItems: () => request<{ items: OrganizerItem[] }>("/items?limit=1000"),
  createItem: (item: Candidate) => request<OrganizerItem>("/items", { method: "POST", body: JSON.stringify(item) }),
  updateItem: (item: OrganizerItem) => request<OrganizerItem>(`/items/${encodeURIComponent(item.id)}`, { method: "PUT", body: JSON.stringify(item) }),
  completeItem: (id: string) => request<OrganizerItem>(`/items/${encodeURIComponent(id)}/complete`, { method: "POST", body: "{}" }),
  cancelItem: (id: string) => request<OrganizerItem>(`/items/${encodeURIComponent(id)}/cancel`, { method: "POST", body: "{}" }),
  reopenItem: (id: string) => request<OrganizerItem>(`/items/${encodeURIComponent(id)}/reopen`, { method: "POST", body: "{}" }),
  archiveItem: (id: string) => request<OrganizerItem>(`/items/${encodeURIComponent(id)}/archive`, { method: "POST", body: "{}" }),
  listItemEvents: (id: string) => request<{ events: ItemEvent[] }>(`/items/${encodeURIComponent(id)}/events?limit=200`),
  listEvents: () => request<{ events: ItemEvent[] }>("/events?limit=1000"),
  snoozeItem: (id: string, minutes = 10) =>
    request<OrganizerItem>(`/items/${encodeURIComponent(id)}/snooze`, { method: "POST", body: JSON.stringify({ minutes }) }),
  listProjects: () => request<{ projects: Project[] }>("/projects"),
  createProject: (project: Pick<Project, "name"> & Partial<Project>) => request<Project>("/projects", { method: "POST", body: JSON.stringify(project) }),
  updateProject: (project: Project) => request<Project>(`/projects/${encodeURIComponent(project.id)}`, { method: "PUT", body: JSON.stringify(project) }),
  listMemories: () => request<{ memories: Memory[] }>("/memories"),
  createMemory: (memory: Pick<Memory, "content"> & Partial<Memory>) => request<Memory>("/memories", { method: "POST", body: JSON.stringify(memory) }),
  updateMemory: (memory: Memory) => request<Memory>(`/memories/${encodeURIComponent(memory.id)}`, { method: "PUT", body: JSON.stringify(memory) }),
  vapidKey: () => request<{ enabled: boolean; publicKey: string }>("/push/vapid-key"),
  subscribe: (subscription: PushSubscriptionJSON) =>
    request("/push/subscriptions", { method: "POST", body: JSON.stringify(subscription) }),
}
