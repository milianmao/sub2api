/**
 * Admin Microsoft Emails API endpoints
 * Handles Microsoft email account management for administrators
 */

import { apiClient } from '../client'

export type MicrosoftEmailStatus = 'active' | 'error' | 'unchecked' | 'inactive' | string

export interface MicrosoftEmailAccount {
  id: number
  email: string
  password?: string
  client_id: string
  refresh_token?: string
  status: MicrosoftEmailStatus
  last_check_at?: string | null
  last_fetch_at?: string | null
  last_error?: string | null
  created_at: string
  updated_at: string
}

export type MicrosoftEmailListItem = MicrosoftEmailAccount

export interface MicrosoftEmailListParams {
  page?: number
  page_size?: number
  search?: string
  status?: string
}

export interface MicrosoftEmailListResponse {
  items: MicrosoftEmailListItem[]
  total: number
  page: number
  page_size: number
  total_pages?: number
}

export interface MicrosoftEmailImportRequest {
  content: string
}

export interface MicrosoftEmailImportItem {
  line: number
  email: string
  action: string
  account?: MicrosoftEmailAccount
}

export interface MicrosoftEmailImportError {
  line: number
  email?: string
  error: string
}

export interface MicrosoftEmailImportResult {
  total: number
  created: number
  updated: number
  failed: number
  items: MicrosoftEmailImportItem[]
  errors: MicrosoftEmailImportError[]
}

export interface MicrosoftEmailCheckResult {
  id: number
  email?: string
  status: MicrosoftEmailStatus
  checked_at?: string
  last_error?: string | null
}

export interface MicrosoftEmailBatchCheckResult {
  total: number
  items: MicrosoftEmailCheckResult[]
}

export interface MicrosoftEmailFetchCodeResult {
  email: string
  code: string
  source: string
  subject: string
  from: string
  received_at: string
  snippet: string
  error: string
}

export interface MicrosoftEmailBatchDeleteResult {
  success: boolean
  count: number
}

/**
 * List Microsoft email accounts with pagination and filters.
 */
export async function list(params?: MicrosoftEmailListParams): Promise<MicrosoftEmailListResponse> {
  const { data } = await apiClient.get<MicrosoftEmailListResponse>('/admin/microsoft-emails', {
    params
  })
  return data
}

/**
 * Import Microsoft email accounts from TXT content.
 */
export async function importTXT(payload: MicrosoftEmailImportRequest): Promise<MicrosoftEmailImportResult> {
  const { data } = await apiClient.post<MicrosoftEmailImportResult>('/admin/microsoft-emails/import', payload)
  return data
}

/**
 * Check a single Microsoft email account.
 */
export async function check(id: number): Promise<MicrosoftEmailCheckResult> {
  const { data } = await apiClient.post<MicrosoftEmailCheckResult>(`/admin/microsoft-emails/${id}/check`)
  return data
}

/**
 * Batch check selected Microsoft email accounts.
 */
export async function batchCheck(ids: number[]): Promise<MicrosoftEmailBatchCheckResult> {
  const { data } = await apiClient.post<MicrosoftEmailBatchCheckResult>('/admin/microsoft-emails/batch-check', { ids })
  return data
}

/**
 * Fetch a verification code for a single Microsoft email account.
 */
export async function fetchCode(id: number): Promise<MicrosoftEmailFetchCodeResult> {
  const { data } = await apiClient.post<MicrosoftEmailFetchCodeResult>(`/admin/microsoft-emails/${id}/fetch-code`)
  return data
}

/**
 * Delete a single Microsoft email account.
 */
export async function deleteMicrosoftEmail(id: number): Promise<MicrosoftEmailBatchDeleteResult> {
  const { data } = await apiClient.delete<MicrosoftEmailBatchDeleteResult>(`/admin/microsoft-emails/${id}`)
  return data
}

/**
 * Batch delete selected Microsoft email accounts.
 */
export async function batchDelete(ids: number[]): Promise<MicrosoftEmailBatchDeleteResult> {
  const { data } = await apiClient.post<MicrosoftEmailBatchDeleteResult>('/admin/microsoft-emails/batch-delete', { ids })
  return data
}

export const microsoftEmailsAPI = {
  list,
  importTXT,
  check,
  batchCheck,
  fetchCode,
  delete: deleteMicrosoftEmail,
  batchDelete
}

export default microsoftEmailsAPI
