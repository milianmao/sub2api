import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import MicrosoftEmailsView from '../MicrosoftEmailsView.vue'

const { listMicrosoftEmails, fetchCode } = vi.hoisted(() => ({
  listMicrosoftEmails: vi.fn(),
  fetchCode: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    microsoftEmails: {
      list: listMicrosoftEmails,
      importTXT: vi.fn(),
      check: vi.fn(),
      batchCheck: vi.fn(),
      fetchCode,
      delete: vi.fn(),
      batchDelete: vi.fn()
    }
  }
}))

const accounts = [
  {
    id: 1,
    email: 'account-a@example.com',
    client_id: 'client-a',
    status: 'active',
    last_check_at: null,
    last_fetch_at: null,
    last_error: null,
    created_at: '2026-05-22T00:00:00Z',
    updated_at: '2026-05-22T00:00:00Z'
  },
  {
    id: 2,
    email: 'account-b@example.com',
    client_id: 'client-b',
    status: 'active',
    last_check_at: null,
    last_fetch_at: null,
    last_error: null,
    created_at: '2026-05-22T00:00:00Z',
    updated_at: '2026-05-22T00:00:00Z'
  }
]

const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id" :data-test="'row-' + row.id">
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(resolvePromise => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function mountView() {
  return mount(MicrosoftEmailsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
        DataTable: DataTableStub,
        Pagination: true,
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
        ConfirmDialog: true,
        StatusBadge: true,
        Icon: true
      }
    }
  })
}

describe('admin MicrosoftEmailsView', () => {
  beforeEach(() => {
    listMicrosoftEmails.mockReset()
    fetchCode.mockReset()

    listMicrosoftEmails.mockResolvedValue({
      items: accounts,
      total: accounts.length,
      page: 1,
      page_size: 20
    })
  })

  it('prevents a second fetch-code request while the global result dialog is waiting', async () => {
    const pendingFetch = deferred({
      email: 'account-a@example.com',
      code: '123456',
      source: 'microsoft',
      subject: 'Code',
      from: 'no-reply@example.com',
      received_at: '2026-05-22T00:00:00Z',
      snippet: 'Use 123456',
      error: ''
    })
    fetchCode.mockReturnValueOnce(pendingFetch.promise)

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="row-1"] button:nth-child(2)').trigger('click')
    await wrapper.get('[data-test="row-2"] button:nth-child(2)').trigger('click')

    expect(fetchCode).toHaveBeenCalledTimes(1)
    expect(fetchCode).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('正在获取验证码')

    pendingFetch.resolve({
      email: 'account-a@example.com',
      code: '123456',
      source: 'microsoft',
      subject: 'Code',
      from: 'no-reply@example.com',
      received_at: '2026-05-22T00:00:00Z',
      snippet: 'Use 123456',
      error: ''
    })
    await flushPromises()

    expect(wrapper.text()).toContain('account-a@example.com')
    expect(wrapper.text()).toContain('123456')
  })
})
