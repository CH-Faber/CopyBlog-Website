import { App } from "@capacitor/app"
import { Capacitor } from "@capacitor/core"
import { LocalNotifications } from "@capacitor/local-notifications"
import { SecureStorage } from "@aparajita/capacitor-secure-storage"
import { configureOrganizerApi, organizerApi, type OrganizerItem } from "@/lib/organizer-api"

const apiBase = "https://faberhu.top/api/agenda/v1"
const tokenKey = "organizer-device-token"
const channelId = "agenda_reminders_v1"
const actionTypeId = "agenda_reminder_actions"
const activeStatuses = new Set(["inbox", "todo", "doing"])
let listenersReady = false

export type NativeNotificationStatus = {
  native: boolean
  permission: string
  exactAlarm: string
  scheduled: number
}

export function isNativeAgenda() {
  return Capacitor.isNativePlatform()
}

export async function initializeNativeAgenda() {
  if (!isNativeAgenda()) return
  await SecureStorage.setKeyPrefix("agenda_")
  const stored = await SecureStorage.get(tokenKey, false)
  configureOrganizerApi({
    baseUrl: apiBase,
    deviceName: "一个闪念 Android",
    token: typeof stored === "string" ? stored : undefined,
    saveToken: (token) => SecureStorage.set(tokenKey, token),
    clearToken: async () => { await SecureStorage.remove(tokenKey) },
  })
  await ensureNotificationInfrastructure()
}

async function ensureNotificationInfrastructure() {
  if (!isNativeAgenda()) return
  await LocalNotifications.createChannel({
    id: channelId,
    name: "日程提醒",
    description: "一个闪念中的任务和日程提醒",
    importance: 4,
    visibility: 1,
    lights: true,
    lightColor: "#16A34A",
    vibration: true,
  })
  await LocalNotifications.registerActionTypes({
    types: [{
      id: actionTypeId,
      actions: [
        { id: "complete", title: "完成" },
        { id: "snooze", title: "推迟 10 分钟" },
      ],
    }],
  })
  if (listenersReady) return
  listenersReady = true
  await LocalNotifications.addListener("localNotificationActionPerformed", async ({ actionId, notification }) => {
    const itemId = typeof notification.extra?.itemId === "string" ? notification.extra.itemId : ""
    if (!itemId) return
    try {
      if (actionId === "complete") await organizerApi.completeItem(itemId)
      if (actionId === "snooze") await organizerApi.snoozeItem(itemId, 10)
    } finally {
      window.dispatchEvent(new CustomEvent("agenda-native-refresh", { detail: { itemId } }))
    }
  })
  await App.addListener("appStateChange", ({ isActive }) => {
    if (isActive) window.dispatchEvent(new Event("agenda-native-refresh"))
  })
}

function notificationId(value: string) {
  let hash = 0x811c9dc5
  for (let index = 0; index < value.length; index++) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 0x01000193)
  }
  return (hash >>> 1) || 1
}

function reminderItems(items: OrganizerItem[]) {
  const now = Date.now()
  return items
    .filter((item) => activeStatuses.has(item.status) && item.reminderAt && new Date(item.reminderAt).getTime() > now)
    .sort((a, b) => new Date(a.reminderAt!).getTime() - new Date(b.reminderAt!).getTime())
    .slice(0, 200)
}

export async function syncNativeNotifications(items: OrganizerItem[]) {
  if (!isNativeAgenda()) return { scheduled: 0 }
  await ensureNotificationInfrastructure()
  const permission = await LocalNotifications.checkPermissions()
  if (permission.display !== "granted") return { scheduled: 0 }

  const pending = await LocalNotifications.getPending()
  const agendaPending = pending.notifications.filter((notification) => notification.extra?.agenda === true)
  if (agendaPending.length) {
    await LocalNotifications.cancel({ notifications: agendaPending.map(({ id }) => ({ id })) })
  }

  const used = new Set<number>()
  const notifications = reminderItems(items).map((item) => {
    let id = notificationId(item.id)
    while (used.has(id)) id = id === 2147483647 ? 1 : id + 1
    used.add(id)
    return {
      id,
      title: item.title,
      body: item.description || (item.project ? `项目：${item.project}` : "该处理这件事了"),
      largeBody: item.description || undefined,
      channelId,
      actionTypeId,
      autoCancel: true,
      schedule: { at: new Date(item.reminderAt!), allowWhileIdle: true },
      extra: { agenda: true, itemId: item.id, version: item.version },
    }
  })
  if (notifications.length) await LocalNotifications.schedule({ notifications })
  return { scheduled: notifications.length }
}

export async function enableNativeNotifications(items: OrganizerItem[]) {
  if (!isNativeAgenda()) throw new Error("当前不是 Android 安装版")
  let permission = await LocalNotifications.checkPermissions()
  if (permission.display !== "granted") permission = await LocalNotifications.requestPermissions()
  if (permission.display !== "granted") throw new Error("请在系统设置中允许一个闪念发送通知")
  const result = await syncNativeNotifications(items)
  return { ...await getNativeNotificationStatus(), scheduled: result.scheduled }
}

export async function requestExactAlarmAccess() {
  if (!isNativeAgenda()) return
  const status = await LocalNotifications.checkExactNotificationSetting()
  if (status.exact_alarm !== "granted") await LocalNotifications.changeExactNotificationSetting()
}

export async function sendNativeTestNotification() {
  if (!isNativeAgenda()) throw new Error("当前不是 Android 安装版")
  let permission = await LocalNotifications.checkPermissions()
  if (permission.display !== "granted") permission = await LocalNotifications.requestPermissions()
  if (permission.display !== "granted") throw new Error("通知权限没有开启")
  await ensureNotificationInfrastructure()
  await LocalNotifications.schedule({ notifications: [{
    id: 2147483000,
    title: "一个闪念通知测试",
    body: "本地通知工作正常。",
    channelId,
    autoCancel: true,
    extra: { agenda: true, test: true },
    schedule: { at: new Date(Date.now() + 60_000), allowWhileIdle: true },
  }] })
}

export async function getNativeNotificationStatus(): Promise<NativeNotificationStatus> {
  if (!isNativeAgenda()) return { native: false, permission: "unsupported", exactAlarm: "unsupported", scheduled: 0 }
  const [permission, exact, pending] = await Promise.all([
    LocalNotifications.checkPermissions(),
    LocalNotifications.checkExactNotificationSetting(),
    LocalNotifications.getPending(),
  ])
  return {
    native: true,
    permission: permission.display,
    exactAlarm: exact.exact_alarm,
    scheduled: pending.notifications.filter((notification) => notification.extra?.agenda === true).length,
  }
}
