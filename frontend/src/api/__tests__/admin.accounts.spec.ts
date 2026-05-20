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

  it('generates checkout links through the account checkout endpoint', async () => {
    post.mockResolvedValueOnce({ data: { url: 'https://chatgpt.com/checkout/example' } })

    const result = await accountsAPI.generateCheckoutLink(42)

    expect(post).toHaveBeenCalledWith('/admin/accounts/42/checkout-link')
    expect(result).toEqual({ url: 'https://chatgpt.com/checkout/example' })
  })
})
