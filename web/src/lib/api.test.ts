import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, apiFetch } from './api'

const respond = (status: number, body: unknown, json = true) =>
  vi.fn().mockResolvedValue(
    new Response(json ? JSON.stringify(body) : String(body), {
      status,
      headers: { 'Content-Type': json ? 'application/json' : 'text/plain' },
    }),
  )

afterEach(() => vi.unstubAllGlobals())

describe('apiFetch', () => {
  it('returns parsed JSON on success', async () => {
    vi.stubGlobal('fetch', respond(200, { version: '1.0.0' }))
    await expect(apiFetch('/api/v1/version')).resolves.toEqual({
      version: '1.0.0',
    })
  })

  it('maps API error bodies to ApiError', async () => {
    vi.stubGlobal(
      'fetch',
      respond(404, {
        error: { code: 'not_found', message: 'route not found' },
      }),
    )
    const err = await apiFetch('/api/v1/nope').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({
      status: 404,
      code: 'not_found',
      message: 'route not found',
    })
  })

  it('handles non-JSON error bodies', async () => {
    vi.stubGlobal('fetch', respond(502, 'Bad Gateway', false))
    await expect(apiFetch('/x')).rejects.toMatchObject({
      status: 502,
      code: 'http_error',
    })
  })
})
