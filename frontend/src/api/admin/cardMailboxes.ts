/**
 * Admin card mailbox API endpoints.
 * Sensitive mailbox_url/token/password values are intentionally excluded from frontend types.
 */

import { apiClient } from '../client'

export type CardMailboxFetchStatus = 'success' | 'failed' | string

export interface CardMailboxListItem {
  id: number
  email: string
  last_code?: string
  last_status?: CardMailboxFetchStatus
  last_error?: string | null
  last_fetched_at?: string | null
  created_at: string
  updated_at: string
}

export interface CardMailboxListParams {
  page?: number
  page_size?: number
  search?: string
  status?: string
}

export interface CardMailboxListResponse {
  items: CardMailboxListItem[]
  total: number
  page: number
  page_size: number
  pages?: number
  total_pages?: number
}

export interface CardMailboxImportRequest {
  content: string
}

export interface CardMailboxImportError {
  line: number
  message: string
}

export interface CardMailboxImportResult {
  imported: number
  failed: number
  errors: CardMailboxImportError[]
}

export interface CardMailboxFetchCodeResult {
  email: string
  code: string
  status: CardMailboxFetchStatus
  fetched_at: string
  source: string
  subject: string
  from: string
  received_at: string
  snippet: string
}

export interface CardMailboxDeleteResult {
  success: boolean
  count: number
}

/**
 * List card mailboxes with pagination and filters.
 */
export async function list(params?: CardMailboxListParams): Promise<CardMailboxListResponse> {
  const { data } = await apiClient.get<CardMailboxListResponse>('/admin/card-mailboxes', {
    params
  })
  return data
}

/**
 * Import card mailbox JSONL content.
 */
export async function importJSONL(payload: CardMailboxImportRequest): Promise<CardMailboxImportResult> {
  const { data } = await apiClient.post<CardMailboxImportResult>('/admin/card-mailboxes/import', payload)
  return data
}

/**
 * Fetch a verification code for a single card mailbox.
 */
export async function fetchCode(id: number): Promise<CardMailboxFetchCodeResult> {
  const { data } = await apiClient.post<CardMailboxFetchCodeResult>(`/admin/card-mailboxes/${id}/fetch-code`)
  return data
}

/**
 * Delete a single card mailbox.
 */
export async function deleteCardMailbox(id: number): Promise<CardMailboxDeleteResult> {
  const { data } = await apiClient.delete<CardMailboxDeleteResult>(`/admin/card-mailboxes/${id}`)
  return data
}

export const cardMailboxesAPI = {
  list,
  importJSONL,
  fetchCode,
  delete: deleteCardMailbox
}

export default cardMailboxesAPI
