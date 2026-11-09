import { describe, expect, it } from 'vitest'
import { decodeKey, deviceName, pushSupport } from './push'

describe('deviceName', () => {
  it.each([
    ['Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:132.0) Gecko/20100101 Firefox/132.0', 'Firefox on macOS'],
    [
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36 Edg/130.0',
      'Edge on Windows',
    ],
    [
      'Mozilla/5.0 (Linux; Android 15) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Mobile Safari/537.36',
      'Chrome on Android',
    ],
    [
      'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1',
      'Safari on iOS',
    ],
    ['curl/8.0', 'Browser'],
  ])('%s', (ua, want) => expect(deviceName(ua)).toBe(want))
})

describe('decodeKey', () => {
  it('decodes unpadded base64url', () => {
    expect(Array.from(decodeKey('_-8'))).toEqual([255, 239])
    expect(decodeKey('AAAA').byteLength).toBe(3)
  })
})

describe('pushSupport', () => {
  const nav = (ua: string, extra: object = {}) => ({ userAgent: ua, maxTouchPoints: 0, ...extra }) as unknown as Navigator
  const win = (props: object, standalone = false) =>
    ({
      ...props,
      matchMedia: () => ({ matches: standalone }),
    }) as unknown as Window

  it('is supported with service workers, PushManager and Notification', () => {
    expect(pushSupport(nav('x', { serviceWorker: {} }), win({ PushManager: {}, Notification: {} }))).toBe('supported')
  })
  it('asks iOS Safari to install the app first', () => {
    expect(pushSupport(nav('Mozilla/5.0 (iPhone; CPU iPhone OS 18_1)'), win({}))).toBe('needs-install')
  })
  it('is unsupported elsewhere', () => {
    expect(pushSupport(nav('Mozilla/5.0 (X11; Linux)'), win({}))).toBe('unsupported')
  })
})
