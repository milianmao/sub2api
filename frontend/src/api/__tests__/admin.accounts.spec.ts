import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    post
  }
}))

import { accountsAPI } from '@/api/admin/accounts'

describe('admin accounts api', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('imports ChatGPT sessions through the dedicated admin endpoint', async () => {
    const payload = {
      content: '{"accessToken":"token"}',
      contents: ['{"accessToken":"token-from-file"}'],
      group_ids: [1, 2],
      proxy_id: 9,
      concurrency: 3,
      priority: 50,
      rate_multiplier: 1,
      load_factor: 2,
      expires_at: 1735689600,
      auto_pause_on_expired: true
    }
    post.mockResolvedValueOnce({
      data: {
        total: 2,
        created: 2,
        failed: 0
      }
    })

    const result = await accountsAPI.importChatGPTSession(payload)

    expect(post).toHaveBeenCalledWith('/admin/accounts/import/chatgpt-session', payload)
    expect(result).toEqual({
      total: 2,
      created: 2,
      failed: 0
    })
  })

  it('generates checkout links through the account checkout endpoint as text', async () => {
    post.mockResolvedValueOnce({ data: 'https://chatgpt.com/checkout/example' })

    const result = await accountsAPI.generateCheckoutLink(42)

    expect(post).toHaveBeenCalledWith('/admin/accounts/42/checkout-link', undefined, { responseType: 'text' })
    expect(result).toBe('https://chatgpt.com/checkout/example')
  })

  it('runs account liveness checks through the admin batch endpoint', async () => {
    const payload = {
      scope: 'selected' as const,
      account_ids: [1, 2],
      concurrency: 5
    }
    post.mockResolvedValueOnce({
      data: {
        total: 2,
        completed: 2,
        success: 1,
        failed: 1,
        skipped: 0,
        average_latency_ms: 120,
        by_platform: {},
        failure_reasons: {},
        items: []
      }
    })

    const result = await accountsAPI.livenessCheck(payload)

    expect(post).toHaveBeenCalledWith('/admin/accounts/liveness-check', payload, { timeout: 180000 })
    expect(result.total).toBe(2)
    expect(result.success).toBe(1)
  })
})
