import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  generateCheckoutLink,
  livenessCheck,
  copyToClipboard,
  showSuccess,
  showError,
  windowOpen
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  generateCheckoutLink: vi.fn(),
  livenessCheck: vi.fn(),
  copyToClipboard: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  windowOpen: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn(),
      generateCheckoutLink,
      livenessCheck
    },
    proxies: {
      getAll: getAllProxies
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token'
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
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

const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <div data-test="data-table">
      <template v-for="row in data" :key="row.id">
        <slot name="cell-name" :row="row" :value="row.name" />
      </template>
    </div>
  `
}

const AccountBulkActionsBarStub = {
  props: ['selectedIds'],
  emits: ['edit-filtered'],
  template: '<button data-test="edit-filtered" @click="$emit(\'edit-filtered\')">edit filtered</button>'
}

const BulkEditAccountModalStub = {
  props: ['show', 'target'],
  template: '<div data-test="bulk-edit-modal" :data-show="String(show)" :data-target-mode="target?.mode ?? \'\'"></div>'
}

const ImportChatGPTSessionModalStub = {
  props: ['show'],
  template: '<div data-test="chatgpt-session-modal" :data-show="String(show)"></div>'
}

const AccountActionMenuStub = {
  emits: ['generate-checkout-link', 'copy-access-token'],
  template: `
    <div>
      <button data-test="generate-checkout-link" @click="$emit('generate-checkout-link', { id: 42 })">generate</button>
      <button data-test="copy-access-token" @click="$emit('copy-access-token', { id: 42, credentials: { access_token: 'account-access-token' } })">copy</button>
    </div>
  `
}

describe('admin AccountsView bulk edit scope', () => {
  beforeEach(() => {
    localStorage.clear()

    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    generateCheckoutLink.mockReset()
    livenessCheck.mockReset()
    copyToClipboard.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    windowOpen.mockReset()
    vi.stubGlobal('open', windowOpen)

    listAccounts.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 0
    })
    listWithEtag.mockResolvedValue({
      notModified: true,
      etag: null,
      data: null
    })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    generateCheckoutLink.mockResolvedValue('https://chatgpt.com/checkout/example')
    livenessCheck.mockResolvedValue({
      total: 0,
      completed: 0,
      success: 0,
      failed: 0,
      skipped: 0,
      average_latency_ms: 0,
      by_platform: {},
      failure_reasons: {},
      items: []
    })
    copyToClipboard.mockResolvedValue(true)
  })

  it('opens bulk edit in filtered-results mode from the bulk actions dropdown', async () => {
    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountActionMenu: AccountActionMenuStub,
          ImportChatGPTSessionModal: ImportChatGPTSessionModalStub,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: BulkEditAccountModalStub,
          PlatformTypeBadge: true,
          AccountCapacityCell: true,
          AccountStatusIndicator: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountUsageCell: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-test="edit-filtered"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="bulk-edit-modal"]').attributes('data-show')).toBe('true')
    expect(wrapper.get('[data-test="bulk-edit-modal"]').attributes('data-target-mode')).toBe('filtered')
  })

  it('shows the ChatGPT Session import entry in more actions and opens its modal', async () => {
    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountActionMenu: AccountActionMenuStub,
          ImportChatGPTSessionModal: ImportChatGPTSessionModalStub,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: BulkEditAccountModalStub,
          PlatformTypeBadge: true,
          AccountCapacityCell: true,
          AccountStatusIndicator: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountUsageCell: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    await wrapper.get('button[title="admin.accounts.moreActions"]').trigger('click')

    const openImportButton = wrapper
      .findAll('button')
      .find((node) => node.text().includes('admin.accounts.chatgptSessionImportEntry'))

    expect(openImportButton).toBeTruthy()

    await openImportButton!.trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="chatgpt-session-modal"]').attributes('data-show')).toBe('true')
  })

  it('generates and opens an OpenAI checkout link from the account action menu', async () => {
    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountActionMenu: AccountActionMenuStub,
          ImportChatGPTSessionModal: ImportChatGPTSessionModalStub,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: BulkEditAccountModalStub,
          PlatformTypeBadge: true,
          AccountCapacityCell: true,
          AccountStatusIndicator: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountUsageCell: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-test="generate-checkout-link"]').trigger('click')
    await flushPromises()

    expect(generateCheckoutLink).toHaveBeenCalledWith(42)
    expect(windowOpen).toHaveBeenCalledWith(
      'https://chatgpt.com/checkout/example',
      '_blank',
      'noopener,noreferrer'
    )
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
  })

  it('shows a clear error when checkout link generation fails', async () => {
    generateCheckoutLink.mockRejectedValueOnce({
      response: {
        data: {
          message: 'Checkout unavailable for this account'
        }
      }
    })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountActionMenu: AccountActionMenuStub,
          ImportChatGPTSessionModal: ImportChatGPTSessionModalStub,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: BulkEditAccountModalStub,
          PlatformTypeBadge: true,
          AccountCapacityCell: true,
          AccountStatusIndicator: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountUsageCell: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-test="generate-checkout-link"]').trigger('click')
    await flushPromises()

    expect(windowOpen).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('Checkout unavailable for this account')
  })

  it('copies an account access token from the account action menu', async () => {
    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountActionMenu: AccountActionMenuStub,
          ImportChatGPTSessionModal: ImportChatGPTSessionModalStub,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: BulkEditAccountModalStub,
          PlatformTypeBadge: true,
          AccountCapacityCell: true,
          AccountStatusIndicator: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountUsageCell: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-test="copy-access-token"]').trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('account-access-token', 'admin.accounts.accessTokenCopied')
    expect(showError).not.toHaveBeenCalled()
  })

  it('shows a clear error when the account has no access token to copy', async () => {
    const AccountActionMenuWithoutTokenStub = {
      emits: ['copy-access-token'],
      template: '<button data-test="copy-access-token" @click="$emit(\'copy-access-token\', { id: 42, credentials: {} })">copy</button>'
    }

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountActionMenu: AccountActionMenuWithoutTokenStub,
          ImportChatGPTSessionModal: ImportChatGPTSessionModalStub,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: BulkEditAccountModalStub,
          PlatformTypeBadge: true,
          AccountCapacityCell: true,
          AccountStatusIndicator: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountUsageCell: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-test="copy-access-token"]').trigger('click')
    await flushPromises()

    expect(copyToClipboard).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.accounts.accessTokenUnavailable')
  })

  it('copies the account email when clicking the email in the name cell', async () => {
    listAccounts.mockResolvedValueOnce({
      items: [
        {
          id: 7,
          name: 'OpenAI Account',
          platform: 'openai',
          type: 'shared',
          schedulable: true,
          status: 'active',
          extra: {
            email_address: 'owner@example.com'
          },
          credentials: {}
        }
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountActionMenu: AccountActionMenuStub,
          ImportChatGPTSessionModal: ImportChatGPTSessionModalStub,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: BulkEditAccountModalStub,
          PlatformTypeBadge: true,
          AccountCapacityCell: true,
          AccountStatusIndicator: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountUsageCell: true,
          Icon: true
        }
      }
    })

    await flushPromises()

    const emailButton = wrapper.get('[data-test="account-email-copy"]')
    await emailButton.trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('owner@example.com', 'admin.accounts.emailCopied')
  })

  it('opens account liveness check modal from more actions and reloads after completion', async () => {
    listAccounts.mockResolvedValueOnce({
      items: [],
      total: 12,
      page: 1,
      page_size: 20,
      pages: 1
    })
    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountActionMenu: AccountActionMenuStub,
          ImportChatGPTSessionModal: ImportChatGPTSessionModalStub,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          AccountLivenessCheckModal: {
            props: ['show', 'filteredCount'],
            emits: ['completed'],
            template: '<div data-test="liveness-modal" :data-show="String(show)" :data-filtered-count="String(filteredCount)"><button data-test="liveness-complete" @click="$emit(\'completed\')">complete</button></div>'
          },
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: BulkEditAccountModalStub,
          PlatformTypeBadge: true,
          AccountCapacityCell: true,
          AccountStatusIndicator: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountUsageCell: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    await wrapper.get('button[title="admin.accounts.moreActions"]').trigger('click')
    await wrapper.get('[data-test="open-liveness-check"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="liveness-modal"]').attributes('data-show')).toBe('true')
    expect(wrapper.get('[data-test="liveness-modal"]').attributes('data-filtered-count')).toBe('12')

    const callsBeforeComplete = listAccounts.mock.calls.length
    await wrapper.get('[data-test="liveness-complete"]').trigger('click')
    await flushPromises()

    expect(listAccounts.mock.calls.length).toBeGreaterThan(callsBeforeComplete)
  })

  it('maps paused status filter to unschedulable when loading filtered results', async () => {
    listAccounts.mockResolvedValueOnce({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 0
    })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountActionMenu: AccountActionMenuStub,
          ImportChatGPTSessionModal: ImportChatGPTSessionModalStub,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: BulkEditAccountModalStub,
          PlatformTypeBadge: true,
          AccountCapacityCell: true,
          AccountStatusIndicator: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountUsageCell: true,
          Icon: true
        }
      }
    })

    await flushPromises()

    ;(wrapper.vm as any).params.status = 'paused'
    await (wrapper.vm as any).openBulkEditFiltered()

    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      100,
      expect.objectContaining({
        status: 'unschedulable'
      })
    )
  })

  it('maps paused status filter to unschedulable for the main account list request', async () => {
    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountActionMenu: AccountActionMenuStub,
          ImportChatGPTSessionModal: ImportChatGPTSessionModalStub,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: BulkEditAccountModalStub,
          PlatformTypeBadge: true,
          AccountCapacityCell: true,
          AccountStatusIndicator: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountUsageCell: true,
          Icon: true
        }
      }
    })

    await flushPromises()

    ;(wrapper.vm as any).params.status = 'paused'
    await (wrapper.vm as any).reload()

    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({
        status: 'unschedulable'
      }),
      expect.anything()
    )
  })
})
