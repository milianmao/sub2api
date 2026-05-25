import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, deleteRequest } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  deleteRequest: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
    post,
    delete: deleteRequest
  }
}))

import { adminAPI, cardMailboxesAPI } from '@/api/admin'

describe('admin card mailboxes api', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    deleteRequest.mockReset()
  })

  it('mounts card mailbox API under adminAPI without batch helpers', () => {
    expect(adminAPI.cardMailboxes).toBe(cardMailboxesAPI)
    expect('batchFetchCode' in cardMailboxesAPI).toBe(false)
    expect('batchDelete' in cardMailboxesAPI).toBe(false)
    expect('batchFetchCode' in adminAPI.cardMailboxes).toBe(false)
    expect('batchDelete' in adminAPI.cardMailboxes).toBe(false)
  })

  it('lists card mailboxes with backend pagination and filters', async () => {
    get.mockResolvedValueOnce({
      data: {
        items: [],
        total: 0,
        page: 2,
        page_size: 10
      }
    })

    const result = await cardMailboxesAPI.list({ page: 2, page_size: 10, search: 'card', status: 'success' })

    expect(get).toHaveBeenCalledWith('/admin/card-mailboxes', {
      params: { page: 2, page_size: 10, search: 'card', status: 'success' }
    })
    expect(result).toEqual({ items: [], total: 0, page: 2, page_size: 10 })
  })

  it('imports JSONL content through the dedicated import endpoint', async () => {
    const payload = { content: '{"email":"user@example.com","mailbox_url":"https://mail.example.com/inbox"}' }
    post.mockResolvedValueOnce({ data: { imported: 1, failed: 0, errors: [] } })

    const result = await cardMailboxesAPI.importJSONL(payload)

    expect(post).toHaveBeenCalledWith('/admin/card-mailboxes/import', payload)
    expect(result).toEqual({ imported: 1, failed: 0, errors: [] })
  })

  it('fetches a code for a single card mailbox only', async () => {
    const response = {
      email: 'user@example.com',
      code: '123456',
      status: 'success',
      fetched_at: '2026-05-24T00:00:00Z'
    }
    post.mockResolvedValueOnce({ data: response })

    const result = await cardMailboxesAPI.fetchCode(7)

    expect(post).toHaveBeenCalledWith('/admin/card-mailboxes/7/fetch-code')
    expect(result).toEqual(response)
  })

  it('deletes a single card mailbox', async () => {
    deleteRequest.mockResolvedValueOnce({ data: { success: true, count: 1 } })

    const result = await cardMailboxesAPI.delete(7)

    expect(deleteRequest).toHaveBeenCalledWith('/admin/card-mailboxes/7')
    expect(result).toEqual({ success: true, count: 1 })
  })
})
