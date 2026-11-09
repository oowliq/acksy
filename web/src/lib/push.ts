import { api, type PushDevice, type PushDeviceRequest } from './api'

/**
 * Web Push on this device. The browser holds one PushSubscription per
 * service worker; the server holds the matching device record. We remember
 * the record's ID locally to tell "this device" apart from the others.
 */

const deviceKey = 'acksy-push-device'
const swURL = '/sw.js'

export type PushSupport = 'supported' | 'unsupported' | 'needs-install'

/** Whether this browser can receive Web Push. iOS needs the Home Screen app. */
export function pushSupport(nav: Navigator = navigator, win: Window = window): PushSupport {
  const ok = 'serviceWorker' in nav && 'PushManager' in win && 'Notification' in win
  if (ok) return 'supported'
  const ios = /iPad|iPhone|iPod/.test(nav.userAgent) || (nav.userAgent.includes('Mac') && nav.maxTouchPoints > 1)
  const standalone = win.matchMedia?.('(display-mode: standalone)').matches
  return ios && !standalone ? 'needs-install' : 'unsupported'
}

export function storedDeviceId(): string | null {
  try {
    return localStorage.getItem(deviceKey)
  } catch {
    return null
  }
}

function storeDeviceId(id: string | null) {
  try {
    if (id) localStorage.setItem(deviceKey, id)
    else localStorage.removeItem(deviceKey)
  } catch {
    // private mode: the device is still subscribed, only the label is lost
  }
}

/** Decodes an unpadded base64url VAPID key for PushManager.subscribe(). */
export function decodeKey(b64url: string): Uint8Array<ArrayBuffer> {
  const b64 = b64url.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - (b64url.length % 4)) % 4)
  const raw = atob(b64)
  const out = new Uint8Array(new ArrayBuffer(raw.length))
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i)
  return out
}

function sameKey(a: ArrayBuffer | null | undefined, b: Uint8Array): boolean {
  if (!a || a.byteLength !== b.byteLength) return false
  const av = new Uint8Array(a)
  return av.every((v, i) => v === b[i])
}

/** A readable default name, e.g. "Firefox on macOS". */
export function deviceName(ua: string): string {
  const browser = /Edg\//.test(ua)
    ? 'Edge'
    : /Firefox\//.test(ua)
      ? 'Firefox'
      : /Chrome\//.test(ua)
        ? 'Chrome'
        : /Safari\//.test(ua)
          ? 'Safari'
          : 'Browser'
  const os = /iPhone|iPad|iPod/.test(ua)
    ? 'iOS'
    : /Android/.test(ua)
      ? 'Android'
      : /Mac OS X|Macintosh/.test(ua)
        ? 'macOS'
        : /Windows/.test(ua)
          ? 'Windows'
          : /Linux/.test(ua)
            ? 'Linux'
            : ''
  return os ? `${browser} on ${os}` : browser
}

async function browserSubscription(): Promise<PushSubscription | null> {
  const reg = await navigator.serviceWorker.getRegistration('/')
  return reg ? reg.pushManager.getSubscription() : null
}

/**
 * Finds this device among the server's records. A browser subscription the
 * server does not know (e.g. local storage was cleared) is re-registered.
 */
export async function currentDevice(devices: PushDevice[], minPriority = 3): Promise<PushDevice | null> {
  const sub = await browserSubscription()
  if (!sub) {
    storeDeviceId(null)
    return null
  }
  const known = devices.find((d) => d.id === storedDeviceId())
  if (known) return known
  const device = await api.subscribePush(requestBody(sub, minPriority))
  storeDeviceId(device.id)
  return device
}

/** The server request for a browser subscription. */
function requestBody(sub: PushSubscription, minPriority: number): PushDeviceRequest {
  const json = sub.toJSON()
  return {
    endpoint: json.endpoint ?? sub.endpoint,
    keys: { p256dh: json.keys?.p256dh ?? '', auth: json.keys?.auth ?? '' },
    name: deviceName(navigator.userAgent),
    min_priority: minPriority,
  }
}

export class PushPermissionError extends Error {}

/** Asks for permission, subscribes the browser and registers the device. */
export async function enablePush(publicKey: string, minPriority: number): Promise<PushDevice> {
  const permission = await Notification.requestPermission()
  if (permission !== 'granted') throw new PushPermissionError(permission)
  const reg = await navigator.serviceWorker.register(swURL, { scope: '/' })
  await navigator.serviceWorker.ready
  const key = decodeKey(publicKey)
  let sub = await reg.pushManager.getSubscription()
  if (sub && !sameKey(sub.options.applicationServerKey, key)) {
    // Subscribed for another server key (keys were rotated): start over.
    await sub.unsubscribe()
    sub = null
  }
  sub ??= await reg.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: key,
  })
  const device = await api.subscribePush(requestBody(sub, minPriority))
  storeDeviceId(device.id)
  return device
}

/** Unsubscribes the browser and forgets the device on the server. */
export async function disablePush(deviceId: string): Promise<void> {
  const sub = await browserSubscription()
  await sub?.unsubscribe()
  storeDeviceId(null)
  await api.deletePushDevice(deviceId)
}

/** Registers the service worker early so clicks on pushes can focus the app. */
export function registerServiceWorker() {
  if (pushSupport() !== 'supported') return
  navigator.serviceWorker.register(swURL, { scope: '/' }).catch(() => {
    // not fatal: push simply stays unavailable
  })
}
