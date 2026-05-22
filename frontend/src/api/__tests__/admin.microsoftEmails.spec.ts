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

import { adminAPI, microsoftEmailsAPI } from '@/api/admin'

describe('admin microsoft emails api', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    deleteRequest.mockReset()
  })

  it('mounts microsoft email API under adminAPI without batch fetch-code helper', () => {
    expect(adminAPI.microsoftEmails).toBe(microsoftEmailsAPI)
    expect('batchFetchCode' in microsoftEmailsAPI).toBe(false)
    expect('batchFetchCode' in adminAPI.microsoftEmails).toBe(false)
  })

  it('lists microsoft email accounts with backend pagination and filters', async () => {
    get.mockResolvedValueOnce({
      data: {
        items: [],
        total: 0,
        page: 2,
        page_size: 10
      }
    })

    const result = await microsoftEmailsAPI.list({ page: 2, page_size: 10, search: 'outlook', status: 'active' })

    expect(get).toHaveBeenCalledWith('/admin/microsoft-emails', {
      params: { page: 2, page_size: 10, search: 'outlook', status: 'active' }
    })
    expect(result).toEqual({ items: [], total: 0, page: 2, page_size: 10 })
  })

  it('imports TXT content through the dedicated import endpoint', async () => {
    const payload = { content: 'user@example.com----password----client-id----refresh-token' }
    post.mockResolvedValueOnce({ data: { total: 1, created: 1, updated: 0, failed: 0, items: [], errors: [] } })

    const result = await microsoftEmailsAPI.importTXT(payload)

    expect(post).toHaveBeenCalledWith('/admin/microsoft-emails/import', payload)
    expect(result).toEqual({ total: 1, created: 1, updated: 0, failed: 0, items: [], errors: [] })
  })

  it('checks a single microsoft email account', async () => {
    post.mockResolvedValueOnce({ data: { id: 7, email: 'user@example.com', status: 'active', checked_at: '2026-05-22T00:00:00Z' } })

    const result = await microsoftEmailsAPI.check(7)

    expect(post).toHaveBeenCalledWith('/admin/microsoft-emails/7/check')
    expect(result).toEqual({ id: 7, email: 'user@example.com', status: 'active', checked_at: '2026-05-22T00:00:00Z' })
  })

  it('batch checks selected microsoft email account ids', async () => {
    post.mockResolvedValueOnce({ data: { total: 2, items: [] } })

    const result = await microsoftEmailsAPI.batchCheck([1, 2])

    expect(post).toHaveBeenCalledWith('/admin/microsoft-emails/batch-check', { ids: [1, 2] })
    expect(result).toEqual({ total: 2, items: [] })
  })

  it('fetches a code for a single microsoft email account only', async () => {
    const response = {
      email: 'user@example.com',
      code: '123456',
      source: 'microsoft',
      subject: 'Your code',
      from: 'no-reply@example.com',
      received_at: '2026-05-22T00:00:00Z',
      snippet: 'Use 123456'
    }
    post.mockResolvedValueOnce({ data: response })

    const result = await microsoftEmailsAPI.fetchCode(7)

    expect(post).toHaveBeenCalledWith('/admin/microsoft-emails/7/fetch-code')
    expect(result).toEqual(response)
  })

  it('deletes a single microsoft email account', async () => {
    deleteRequest.mockResolvedValueOnce({ data: { success: true, count: 1 } })

    const result = await microsoftEmailsAPI.delete(7)

    expect(deleteRequest).toHaveBeenCalledWith('/admin/microsoft-emails/7')
    expect(result).toEqual({ success: true, count: 1 })
  })

  it('batch deletes selected microsoft email account ids', async () => {
    post.mockResolvedValueOnce({ data: { success: true, count: 2 } })

    const result = await microsoftEmailsAPI.batchDelete([1, 2])

    expect(post).toHaveBeenCalledWith('/admin/microsoft-emails/batch-delete', { ids: [1, 2] })
    expect(result).toEqual({ success: true, count: 2 })
  })
})
