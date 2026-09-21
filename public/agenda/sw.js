const CACHE_NAME = "faber-agenda-v1"
const APP_SHELL = ["/agenda/", "/agenda/manifest.webmanifest", "/icon.svg", "/apple-icon.png"]

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(CACHE_NAME).then((cache) => cache.addAll(APP_SHELL)))
  self.skipWaiting()
})

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) => Promise.all(keys.filter((key) => key !== CACHE_NAME).map((key) => caches.delete(key)))),
  )
  self.clients.claim()
})

self.addEventListener("fetch", (event) => {
  const request = event.request
  if (request.method !== "GET") return

  const url = new URL(request.url)
  if (url.origin !== self.location.origin || url.pathname.startsWith("/api/organizer/")) return

  if (request.mode === "navigate" && url.pathname.startsWith("/agenda")) {
    event.respondWith(
      fetch(request)
        .then((response) => {
          if (response.ok) {
            const copy = response.clone()
            event.waitUntil(caches.open(CACHE_NAME).then((cache) => cache.put("/agenda/", copy)))
          }
          return response
        })
        .catch(() => caches.match("/agenda/")),
    )
    return
  }

  event.respondWith(
    caches.match(request).then((cached) => {
      if (cached) return cached
      return fetch(request).then((response) => {
        if (response.ok && ["style", "script", "image", "font"].includes(request.destination)) {
          const copy = response.clone()
          event.waitUntil(caches.open(CACHE_NAME).then((cache) => cache.put(request, copy)))
        }
        return response
      })
    }),
  )
})

self.addEventListener("push", (event) => {
  let payload = {}
  try {
    payload = event.data ? event.data.json() : {}
  } catch {
    payload = { body: event.data ? event.data.text() : "你有一个待办事项" }
  }

  const title = payload.title || "一个闪念 · 事项提醒"
  event.waitUntil(
    self.registration.showNotification(title, {
      body: payload.body || "你有一个待办事项",
      icon: "/apple-icon.png",
      badge: "/favicon-96x96.png",
      tag: payload.itemId ? `organizer-${payload.itemId}` : "organizer-reminder",
      renotify: true,
      data: { url: payload.url || "/agenda/", itemId: payload.itemId || "" },
      actions: Array.isArray(payload.actions) ? payload.actions : [],
    }),
  )
})

async function mutateItem(itemId, action) {
  if (!itemId) return false
  const suffix = action === "complete" ? "complete" : "snooze"
  const body = action === "complete" ? "{}" : JSON.stringify({ minutes: 10 })
  const response = await fetch(`/api/organizer/v1/items/${encodeURIComponent(itemId)}/${suffix}`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body,
  })
  return response.ok
}

self.addEventListener("notificationclick", (event) => {
  event.notification.close()
  const data = event.notification.data || {}
  event.waitUntil((async () => {
    if (event.action === "complete" || event.action === "snooze") {
      const succeeded = await mutateItem(data.itemId, event.action).catch(() => false)
      if (succeeded) return
    }

    const target = new URL(data.url || "/agenda/", self.location.origin).href
    const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true })
    for (const client of windows) {
      if ("focus" in client) {
        if ("navigate" in client) await client.navigate(target)
        return client.focus()
      }
    }
    return self.clients.openWindow(target)
  })())
})
