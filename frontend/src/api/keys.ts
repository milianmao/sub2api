/**
 * API Keys management endpoints
 * Handles CRUD operations for user API keys
 */

import { apiClient } from './client'
import type { ApiKey, CreateApiKeyRequest, UpdateApiKeyRequest, PaginatedResponse } from '@/types'

type CreateApiKeyData = Omit<CreateApiKeyRequest, 'name'>

function buildCreatePayload(name: string, data: CreateApiKeyData = {}): CreateApiKeyRequest {
  const payload: CreateApiKeyRequest = { name }
  if (data.group_id !== undefined) {
    payload.group_id = data.group_id
  }
  if (data.group_ids !== undefined) {
    payload.group_ids = data.group_ids
  }
  if (data.custom_key) {
    payload.custom_key = data.custom_key
  }
  if (data.ip_whitelist && data.ip_whitelist.length > 0) {
    payload.ip_whitelist = data.ip_whitelist
  }
  if (data.ip_blacklist && data.ip_blacklist.length > 0) {
    payload.ip_blacklist = data.ip_blacklist
  }
  if (data.quota !== undefined && data.quota > 0) {
    payload.quota = data.quota
  }
  if (data.expires_in_days !== undefined && data.expires_in_days > 0) {
    payload.expires_in_days = data.expires_in_days
  }
  if (data.rate_limit_5h && data.rate_limit_5h > 0) {
    payload.rate_limit_5h = data.rate_limit_5h
  }
  if (data.rate_limit_1d && data.rate_limit_1d > 0) {
    payload.rate_limit_1d = data.rate_limit_1d
  }
  if (data.rate_limit_7d && data.rate_limit_7d > 0) {
    payload.rate_limit_7d = data.rate_limit_7d
  }
  return payload
}

/**
 * List all API keys for current user
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 10)
 * @param filters - Optional filter parameters
 * @param options - Optional request options
 * @returns Paginated list of API keys
 */
export async function list(
  page: number = 1,
  pageSize: number = 10,
  filters?: {
    search?: string
    status?: string
    group_id?: number | string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: {
    signal?: AbortSignal
  }
): Promise<PaginatedResponse<ApiKey>> {
  const { data } = await apiClient.get<PaginatedResponse<ApiKey>>('/keys', {
    params: { page, page_size: pageSize, ...filters },
    signal: options?.signal
  })
  return data
}

/**
 * Get API key by ID
 * @param id - API key ID
 * @returns API key details
 */
export async function getById(id: number): Promise<ApiKey> {
  const { data } = await apiClient.get<ApiKey>(`/keys/${id}`)
  return data
}

/**
 * Create new API key
 * @param nameOrRequest - Key name or complete create payload
 * @param groupId - Optional group ID
 * @param customKey - Optional custom key value
 * @param ipWhitelist - Optional IP whitelist
 * @param ipBlacklist - Optional IP blacklist
 * @param quota - Optional quota limit in USD (0 = unlimited)
 * @param expiresInDays - Optional days until expiry (undefined = never expires)
 * @param rateLimitData - Optional rate limit fields
 * @param groupIds - Optional authorized group IDs
 * @returns Created API key
 */
export async function create(request: CreateApiKeyRequest): Promise<ApiKey>
export async function create(
  name: string,
  groupIdOrGroupIds?: number | number[] | null,
  customKey?: string,
  ipWhitelist?: string[],
  ipBlacklist?: string[],
  quota?: number,
  expiresInDays?: number,
  rateLimitData?: { rate_limit_5h?: number; rate_limit_1d?: number; rate_limit_7d?: number },
  groupIds?: number[]
): Promise<ApiKey>
export async function create(
  nameOrRequest: string | CreateApiKeyRequest,
  groupIdOrGroupIds?: number | number[] | null,
  customKey?: string,
  ipWhitelist?: string[],
  ipBlacklist?: string[],
  quota?: number,
  expiresInDays?: number,
  rateLimitData?: { rate_limit_5h?: number; rate_limit_1d?: number; rate_limit_7d?: number },
  groupIds?: number[]
): Promise<ApiKey> {
  if (typeof nameOrRequest !== 'string') {
    const { data } = await apiClient.post<ApiKey>('/keys', nameOrRequest)
    return data
  }

  const groupId = Array.isArray(groupIdOrGroupIds) ? undefined : groupIdOrGroupIds
  const normalizedGroupIds = groupIds ?? (Array.isArray(groupIdOrGroupIds) ? groupIdOrGroupIds : undefined)
  const payload = buildCreatePayload(nameOrRequest, {
    group_id: groupId,
    group_ids: normalizedGroupIds,
    custom_key: customKey,
    ip_whitelist: ipWhitelist,
    ip_blacklist: ipBlacklist,
    quota,
    expires_in_days: expiresInDays,
    rate_limit_5h: rateLimitData?.rate_limit_5h,
    rate_limit_1d: rateLimitData?.rate_limit_1d,
    rate_limit_7d: rateLimitData?.rate_limit_7d
  })

  const { data } = await apiClient.post<ApiKey>('/keys', payload)
  return data
}

/**
 * Update API key
 * @param id - API key ID
 * @param updates - Fields to update
 * @returns Updated API key
 */
export async function update(id: number, updates: UpdateApiKeyRequest): Promise<ApiKey> {
  const { data } = await apiClient.put<ApiKey>(`/keys/${id}`, updates)
  return data
}

/**
 * Delete API key
 * @param id - API key ID
 * @returns Success confirmation
 */
export async function deleteKey(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/keys/${id}`)
  return data
}

/**
 * Toggle API key status (active/inactive)
 * @param id - API key ID
 * @param status - New status
 * @returns Updated API key
 */
export async function toggleStatus(id: number, status: 'active' | 'inactive'): Promise<ApiKey> {
  return update(id, { status })
}

export const keysAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteKey,
  toggleStatus
}

export default keysAPI
