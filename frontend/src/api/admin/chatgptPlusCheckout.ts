import { apiClient } from '../client'

export type ChatGPTPlusCheckoutProxySource = 'direct' | 'pool' | 'extract_api'

export interface CreateChatGPTPlusCheckoutRequest {
  access_token: string
  proxy_source: ChatGPTPlusCheckoutProxySource
  proxy_id?: number
  extract_api_url?: string
}

export interface ChatGPTPlusCheckoutResponse {
  url: string
}

export async function createCheckoutLink(payload: CreateChatGPTPlusCheckoutRequest): Promise<ChatGPTPlusCheckoutResponse> {
  const { data } = await apiClient.post<ChatGPTPlusCheckoutResponse>('/admin/chatgpt-plus-checkout', payload)
  return data
}

export const chatgptPlusCheckoutAPI = {
  createCheckoutLink
}

export default chatgptPlusCheckoutAPI
