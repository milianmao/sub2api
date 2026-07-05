import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    post
  }
}))

import { chatgptPlusCheckoutAPI } from '@/api/admin/chatgptPlusCheckout'

describe('admin chatgpt plus checkout api', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('creates checkout links through the dedicated admin endpoint', async () => {
    const payload = {
      access_token: 'token',
      proxy_source: 'pool' as const,
      proxy_id: 9
    }
    post.mockResolvedValueOnce({
      data: {
        url: 'https://chatgpt.com/payments/checkout/session-1'
      }
    })

    const result = await chatgptPlusCheckoutAPI.createCheckoutLink(payload)

    expect(post).toHaveBeenCalledWith('/admin/chatgpt-plus-checkout', payload)
    expect(result).toEqual({
      url: 'https://chatgpt.com/payments/checkout/session-1'
    })
  })
})
