import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ChatGPTPlusCheckoutView from '../ChatGPTPlusCheckoutView.vue'

const { createCheckoutLink, getAllWithCount, copyToClipboard, showError } = vi.hoisted(() => ({
  createCheckoutLink: vi.fn(),
  getAllWithCount: vi.fn(),
  copyToClipboard: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    chatgptPlusCheckout: {
      createCheckoutLink
    },
    proxies: {
      getAllWithCount
    }
  }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess: vi.fn()
  })
}))

const ProxySelectorStub = {
  props: ['modelValue', 'proxies'],
  emits: ['update:modelValue'],
  template: `
    <select data-test="proxy-selector" :value="modelValue ?? ''" @change="$emit('update:modelValue', Number($event.target.value))">
      <option v-for="proxy in proxies" :key="proxy.id" :value="proxy.id">{{ proxy.name }}</option>
    </select>
  `
}

function mountView() {
  return mount(ChatGPTPlusCheckoutView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        ProxySelector: ProxySelectorStub
      }
    }
  })
}

describe('admin ChatGPTPlusCheckoutView', () => {
  beforeEach(() => {
    createCheckoutLink.mockReset()
    getAllWithCount.mockReset()
    copyToClipboard.mockReset()
    showError.mockReset()
    localStorage.clear()

    getAllWithCount.mockResolvedValue([
      {
        id: 9,
        name: 'Proxy A',
        protocol: 'http',
        host: '127.0.0.1',
        port: 8080,
        username: null,
        status: 'active',
        account_count: 2,
        created_at: '2026-06-04T00:00:00Z',
        updated_at: '2026-06-04T00:00:00Z'
      }
    ])
  })

  it('does not submit when access token is empty', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('button.btn.btn-primary').trigger('click')

    expect(createCheckoutLink).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('请粘贴 accessToken')
  })

  it('submits direct mode payload', async () => {
    createCheckoutLink.mockResolvedValueOnce({
      url: 'https://chatgpt.com/payments/checkout/direct'
    })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('input[type="password"]').setValue('token-1')
    await wrapper.get('button.btn.btn-primary').trigger('click')
    await flushPromises()

    expect(createCheckoutLink).toHaveBeenCalledWith({
      access_token: 'token-1',
      proxy_source: 'direct'
    })
    expect(wrapper.text()).toContain('https://chatgpt.com/payments/checkout/direct')
  })

  it('submits pool mode payload with proxy id', async () => {
    createCheckoutLink.mockResolvedValueOnce({
      url: 'https://chatgpt.com/payments/checkout/pool'
    })

    localStorage.setItem('chatgpt_plus_checkout_proxy_source', 'pool')
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('input[type="password"]').setValue('token-2')
    await wrapper.get('button.btn.btn-primary').trigger('click')
    await flushPromises()

    expect(createCheckoutLink).toHaveBeenCalledWith({
      access_token: 'token-2',
      proxy_source: 'pool',
      proxy_id: 9
    })
  })

  it('submits extract api payload', async () => {
    createCheckoutLink.mockResolvedValueOnce({
      url: 'https://chatgpt.com/payments/checkout/extract'
    })

    localStorage.setItem('chatgpt_plus_checkout_proxy_source', 'extract_api')
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('input[type="password"]').setValue('token-3')
    await wrapper.get('input[type="url"]').setValue('https://example.com/get-proxy')
    await wrapper.get('button.btn.btn-primary').trigger('click')
    await flushPromises()

    expect(createCheckoutLink).toHaveBeenCalledWith({
      access_token: 'token-3',
      proxy_source: 'extract_api',
      extract_api_url: 'https://example.com/get-proxy'
    })
  })

  it('shows backend business message as failure instead of success url', async () => {
    createCheckoutLink.mockRejectedValueOnce({
      response: {
        data: {
          message: '账号已订阅，无需生成支付链接'
        }
      }
    })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('input[type="password"]').setValue('token-5')
    await wrapper.get('button.btn.btn-primary').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('账号已订阅，无需生成支付链接')
    expect(wrapper.text()).not.toContain('支付长链生成成功')
  })

  it('does not persist access token and persists non-sensitive preferences', async () => {
    localStorage.setItem('chatgpt_plus_checkout_proxy_source', 'extract_api')
    localStorage.setItem('chatgpt_plus_checkout_extract_api_url', 'https://example.com/get-proxy')
    localStorage.setItem('chatgpt_plus_checkout_auto_open', 'true')

    const wrapper = mountView()
    await flushPromises()

    expect((wrapper.get('input[type="password"]').element as HTMLInputElement).value).toBe('')
    expect(localStorage.getItem('chatgpt_plus_checkout_proxy_source')).toBe('extract_api')
    expect(localStorage.getItem('chatgpt_plus_checkout_extract_api_url')).toBe('https://example.com/get-proxy')
    expect(localStorage.getItem('chatgpt_plus_checkout_auto_open')).toBe('true')
    expect(localStorage.getItem('chatgpt_plus_checkout_extract_api_url')).not.toContain('token-4')

    wrapper.unmount()
  })
})
