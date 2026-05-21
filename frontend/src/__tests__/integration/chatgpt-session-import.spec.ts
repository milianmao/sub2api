import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ImportChatGPTSessionModal from '@/components/admin/account/ImportChatGPTSessionModal.vue'

const { importChatGPTSession, showError, showSuccess } = vi.hoisted(() => ({
  importChatGPTSession: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      importChatGPTSession
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const BaseDialogStub = {
  props: ['show', 'title'],
  template: '<div><slot /><slot name="footer" /></div>'
}

const GroupSelectorStub = {
  props: ['modelValue', 'groups'],
  emits: ['update:modelValue'],
  template: '<div data-test="group-selector"></div>'
}

const ProxySelectorStub = {
  props: ['modelValue', 'proxies'],
  emits: ['update:modelValue'],
  template: '<div data-test="proxy-selector"></div>'
}

const mountModal = () =>
  mount(ImportChatGPTSessionModal, {
    props: {
      show: true,
      groups: [],
      proxies: []
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        GroupSelector: GroupSelectorStub,
        ProxySelector: ProxySelectorStub
      }
    }
  })

describe('ImportChatGPTSessionModal', () => {
  beforeEach(() => {
    importChatGPTSession.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
  })

  it('shows an error when neither text nor files are provided', async () => {
    const wrapper = mountModal()

    await wrapper.find('form').trigger('submit')

    expect(showError).toHaveBeenCalledWith('admin.accounts.chatgptSessionImportEmpty')
    expect(importChatGPTSession).not.toHaveBeenCalled()
  })

  it('submits pasted session content and emits imported on full success', async () => {
    importChatGPTSession.mockResolvedValueOnce({
      total: 1,
      created: 1,
      failed: 0,
      items: [{ index: 1, action: 'created', account_id: 101 }]
    })
    const wrapper = mountModal()

    await wrapper.find('textarea').setValue('{"accessToken":"token"}')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importChatGPTSession).toHaveBeenCalledWith({
      content: '{"accessToken":"token"}',
      contents: [],
      notes: undefined,
      group_ids: [],
      proxy_id: null,
      concurrency: 3,
      priority: 50,
      rate_multiplier: 1,
      load_factor: null,
      expires_at: null,
      auto_pause_on_expired: true
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.chatgptSessionImportSuccess')
    expect(wrapper.emitted('imported')).toBeTruthy()
  })

  it('reads multiple files and keeps the result panel open on partial failure', async () => {
    importChatGPTSession.mockResolvedValueOnce({
      total: 2,
      created: 1,
      failed: 1,
      errors: [{ index: 2, name: 'second.json', message: 'missing expiry' }]
    })
    const wrapper = mountModal()
    const input = wrapper.find('input[type="file"]')
    const first = new File(['first'], 'first.json', { type: 'application/json' })
    const second = new File(['second'], 'second.json', { type: 'application/json' })

    Object.defineProperty(first, 'text', {
      value: () => Promise.resolve('{"accessToken":"first"}')
    })
    Object.defineProperty(second, 'text', {
      value: () => Promise.resolve('{"accessToken":"second"}')
    })
    Object.defineProperty(input.element, 'files', {
      value: [first, second]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importChatGPTSession).toHaveBeenCalledWith({
      content: undefined,
      contents: ['{"accessToken":"first"}', '{"accessToken":"second"}'],
      notes: undefined,
      group_ids: [],
      proxy_id: null,
      concurrency: 3,
      priority: 50,
      rate_multiplier: 1,
      load_factor: null,
      expires_at: null,
      auto_pause_on_expired: true
    })
    expect(showError).toHaveBeenCalledWith('admin.accounts.chatgptSessionImportPartial')
    expect(showSuccess).not.toHaveBeenCalled()
    expect(wrapper.emitted('imported')).toBeUndefined()
    expect(wrapper.text()).toContain('admin.accounts.chatgptSessionImportResult')
  })

  it('shows the result panel for full failures without closing the modal', async () => {
    importChatGPTSession.mockResolvedValueOnce({
      total: 1,
      created: 0,
      failed: 1,
      errors: [{ index: 1, name: 'session.json', message: 'missing expiry' }]
    })
    const wrapper = mountModal()

    await wrapper.find('textarea').setValue('{"accessToken":"token"}')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.chatgptSessionImportPartial')
    expect(wrapper.emitted('imported')).toBeUndefined()
    expect(wrapper.text()).toContain('session.json')
  })
})
