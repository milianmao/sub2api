import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountLivenessCheckModal from '../AccountLivenessCheckModal.vue'

const { livenessCheck } = vi.hoisted(() => ({
  livenessCheck: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      livenessCheck
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key
    })
  }
})

const BaseDialogStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: '<div v-if="show" data-test="dialog"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>'
}

const mountModal = (props = {}) => mount(AccountLivenessCheckModal, {
  props: {
    show: true,
    selectedIds: [],
    filters: { platform: '', type: '', status: '', group: '', search: '', privacy_mode: '', sort_by: 'name', sort_order: 'asc' },
    filteredCount: 12,
    ...props
  },
  global: {
    stubs: {
      BaseDialog: BaseDialogStub,
      Icon: true,
      LoadingSpinner: true
    }
  }
})

describe('AccountLivenessCheckModal', () => {
  beforeEach(() => {
    livenessCheck.mockReset()
  })

  it('shows filtered scope when there are no selected accounts', () => {
    const wrapper = mountModal()

    expect(wrapper.text()).toContain('admin.accounts.liveness.scopeFiltered')
    expect(wrapper.text()).toContain('12')
  })

  it('uses selected scope when selected accounts exist', async () => {
    livenessCheck.mockResolvedValueOnce({
      total: 2,
      completed: 2,
      success: 1,
      failed: 1,
      skipped: 0,
      average_latency_ms: 456,
      by_platform: { anthropic: { success: 1, failed: 0, skipped: 0 } },
      failure_reasons: { auth: 1 },
      items: [
        { account_id: 1, account_name: 'ok', platform: 'anthropic', type: 'oauth', result: 'success', latency_ms: 456, status_before: 'error', status_after: 'active', message: '检测成功' },
        { account_id: 2, account_name: 'bad', platform: 'openai', type: 'oauth', result: 'failed', latency_ms: 0, status_before: 'active', status_after: 'error', message: '401 unauthorized' }
      ]
    })
    const wrapper = mountModal({ selectedIds: [1, 2], filteredCount: 99 })

    await wrapper.get('[data-test="start-liveness-check"]').trigger('click')
    await flushPromises()

    expect(livenessCheck).toHaveBeenCalledWith({
      scope: 'selected',
      account_ids: [1, 2],
      concurrency: 5
    })
    expect(wrapper.text()).toContain('1')
    expect(wrapper.text()).toContain('456ms')
    expect(wrapper.text()).toContain('bad')
    expect(wrapper.text()).toContain('401 unauthorized')
  })

  it('emits completed when user finishes after a successful check', async () => {
    livenessCheck.mockResolvedValueOnce({
      total: 1,
      completed: 1,
      success: 1,
      failed: 0,
      skipped: 0,
      average_latency_ms: 100,
      by_platform: {},
      failure_reasons: {},
      items: []
    })
    const wrapper = mountModal({ selectedIds: [1] })

    await wrapper.get('[data-test="start-liveness-check"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="finish-liveness-check"]').trigger('click')

    expect(wrapper.emitted('completed')).toHaveLength(1)
  })

  it('shows request failure and allows retry', async () => {
    livenessCheck.mockRejectedValueOnce(new Error('network failed'))
    const wrapper = mountModal({ selectedIds: [1] })

    await wrapper.get('[data-test="start-liveness-check"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('network failed')
    expect(wrapper.get('[data-test="start-liveness-check"]').text()).toContain('admin.accounts.liveness.retry')
  })
})
