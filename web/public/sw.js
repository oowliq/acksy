/*
 * Acksy service worker: shows Web Push notifications and opens the app on
 * click. It caches nothing; the app always loads from the server.
 * Payload shape: internal/delivery/webpush.Payload.
 */

self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()))

self.addEventListener('push', (event) => {
  let data = {}
  try {
    data = event.data ? event.data.json() : {}
  } catch {
    data = { body: event.data ? event.data.text() : '' }
  }
  const priority = Number(data.priority) || 3
  event.waitUntil(
    self.registration.showNotification(data.title || 'Acksy', {
      body: data.body || '',
      tag: data.tag || undefined,
      // Re-alert when a repeat replaces a shown notification (same tag).
      renotify: Boolean(data.tag && data.renotify),
      // Urgent alerts stay until dismissed; minor ones arrive quietly.
      requireInteraction: priority >= 5,
      silent: priority <= 2,
      timestamp: data.ts || Date.now(),
      icon: '/icon-192.png',
      data: { url: data.url || '/', id: data.id },
    }),
  )
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const target = new URL(event.notification.data?.url || '/', self.location.origin)
  if (target.origin !== self.location.origin) return
  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
      const open = windows.find((c) => new URL(c.url).origin === self.location.origin)
      if (open) {
        await open.focus()
        if ('navigate' in open) await open.navigate(target.href)
        return
      }
      await self.clients.openWindow(target.href)
    })(),
  )
})

// Browsers may rotate a subscription; register the new one with the server
// (the session cookie authenticates the request). The old endpoint is
// removed by the server once its push service reports it gone.
self.addEventListener('pushsubscriptionchange', (event) => {
  event.waitUntil(
    (async () => {
      const options = event.oldSubscription?.options
      const sub = event.newSubscription || (options && (await self.registration.pushManager.subscribe(options)))
      if (!sub) return
      await fetch('/api/v1/push/subscriptions', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...sub.toJSON(), name: 'Renewed subscription' }),
      })
    })(),
  )
})
