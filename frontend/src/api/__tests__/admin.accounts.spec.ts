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

  it('generates checkout links through the account checkout endpoint as text', async () => {
    post.mockResolvedValueOnce({ data: 'https://chatgpt.com/checkout/example' })

    const result = await accountsAPI.generateCheckoutLink(42)

    expect(post).toHaveBeenCalledWith('/admin/accounts/42/checkout-link', undefined, { responseType: 'text' })
    expect(result).toBe('https://chatgpt.com/checkout/example')
  })
})
