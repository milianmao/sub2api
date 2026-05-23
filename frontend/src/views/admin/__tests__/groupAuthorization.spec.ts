import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { AdminGroup, AdminUser, ApiKey, PublicSettings } from '@/types'
import UsersView from '../UsersView.vue'
import GroupsView from '../GroupsView.vue'
import KeysView from '@/views/user/KeysView.vue'
import UserAllowedGroupsModal from '@/components/admin/user/UserAllowedGroupsModal.vue'

const authState = vi.hoisted(() => ({
  user: { id: 1, email: 'root@example.com', username: 'root', role: 'admin' },
  isSimpleMode: false,
}))

const {
  listUsers,
  getAllGroups,
  getBatchUsersUsage,
  listEnabledDefinitions,
  getBatchUserAttributes,
  updateUser,
  updateKey,
  listKeys,
  getDashboardApiKeysUsage,
  getAvailableGroups,
  getUserGroupRates,
  getPublicSettings,
  listGroups,
  createGroupRequest,
  updateGroupRequest,
  getGroupUsageSummary,
  getGroupCapacitySummary,
} = vi.hoisted(() => ({
  listUsers: vi.fn(),
  getAllGroups: vi.fn(),
  getBatchUsersUsage: vi.fn(),
  listEnabledDefinitions: vi.fn(),
  getBatchUserAttributes: vi.fn(),
  updateUser: vi.fn(),
  updateKey: vi.fn(),
  listKeys: vi.fn(),
  getDashboardApiKeysUsage: vi.fn(),
  getAvailableGroups: vi.fn(),
  getUserGroupRates: vi.fn(),
  getPublicSettings: vi.fn(),
  listGroups: vi.fn(),
  createGroupRequest: vi.fn(),
  updateGroupRequest: vi.fn(),
  getGroupUsageSummary: vi.fn(),
  getGroupCapacitySummary: vi.fn(),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn(),
  }),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      list: listUsers,
      update: updateUser,
      toggleStatus: vi.fn(),
      delete: vi.fn(),
    },
    groups: {
      getAll: getAllGroups,
      list: listGroups,
      create: createGroupRequest,
      update: updateGroupRequest,
      getUsageSummary: getGroupUsageSummary,
      getCapacitySummary: getGroupCapacitySummary,
    },
    dashboard: {
      getBatchUsersUsage,
    },
    userAttributes: {
      listEnabledDefinitions,
      getBatchUserAttributes,
    },
  },
}))

vi.mock('@/api', () => ({
  keysAPI: {
    list: listKeys,
    update: updateKey,
    toggleStatus: vi.fn(),
    create: vi.fn(),
    delete: vi.fn(),
  },
  authAPI: {
    getPublicSettings,
  },
  usageAPI: {
    getDashboardApiKeysUsage,
  },
  userGroupsAPI: {
    getAvailable: getAvailableGroups,
    getUserGroupRates,
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const createUser = (overrides: Partial<AdminUser> = {}): AdminUser => ({
  id: 42,
  username: 'scoped-user',
  email: 'scoped@example.com',
  role: 'user',
  level: 3,
  balance: 0,
  concurrency: 1,
  status: 'active',
  allowed_groups: [10],
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: '2026-04-17T00:00:00Z',
  updated_at: '2026-04-17T00:00:00Z',
  notes: '',
  last_active_at: '2026-04-16T02:00:00Z',
  last_used_at: '2026-04-17T02:00:00Z',
  current_concurrency: 0,
  ...overrides,
})

const createGroup = (overrides: Partial<AdminGroup> = {}): AdminGroup => ({
  id: 10,
  name: 'Restricted',
  description: null,
  platform: 'anthropic',
  rate_multiplier: 1,
  rpm_limit: 0,
  is_exclusive: false,
  access_mode: 'restricted',
  min_user_level: 5,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  allow_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '2026-04-17T00:00:00Z',
  updated_at: '2026-04-17T00:00:00Z',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: true,
  sort_order: 10,
  ...overrides,
})

const createKey = (overrides: Partial<ApiKey> = {}): ApiKey => ({
  id: 99,
  user_id: 42,
  key: 'sk-test-key',
  name: 'Test Key',
  group_id: 10,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  quota: 0,
  quota_used: 0,
  expires_at: null,
  created_at: '2026-04-17T00:00:00Z',
  updated_at: '2026-04-17T00:00:00Z',
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null,
  group: createGroup(),
  ...overrides,
})

const publicSettings = {
  api_base_url: 'https://api.example.test',
  custom_endpoints: [],
  hide_ccs_import_button: true,
} as PublicSettings

const IconStub = { template: '<span />' }
const mountedWrappers: VueWrapper[] = []

const mountUsersView = async () => {
  const wrapper = mount(UsersView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>',
        },
        DataTable: {
          props: ['columns', 'data'],
          template: `
            <div>
              <div data-test="columns">{{ columns.map(col => col.key).join(',') }}</div>
              <div v-for="row in data" :key="row.id">
                <slot name="cell-level" :value="row.level" :row="row" />
                <slot name="cell-groups" :row="row" />
                <slot name="cell-actions" :row="row" />
              </div>
            </div>
          `,
        },
        Pagination: true,
        ConfirmDialog: true,
        EmptyState: true,
        GroupBadge: true,
        Select: true,
        UserAttributesConfigModal: true,
        UserConcurrencyCell: true,
        UserCreateModal: true,
        UserEditModal: true,
        UserApiKeysModal: true,
        UserAllowedGroupsModal: true,
        UserBalanceModal: true,
        UserBalanceHistoryModal: true,
        GroupReplaceModal: true,
        Icon: IconStub,
      },
    },
    attachTo: document.body,
  })
  mountedWrappers.push(wrapper)
  await flushPromises()
  await nextTick()
  return wrapper
}

const mountGroupsView = async () => {
  const wrapper = mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>',
        },
        DataTable: {
          props: ['columns', 'data'],
          template: `
            <div>
              <div data-test="columns">{{ columns.map(col => col.key).join(',') }}</div>
              <div v-for="row in data" :key="row.id">
                <slot name="cell-access" :row="row" />
              </div>
            </div>
          `,
        },
        Pagination: true,
        EmptyState: true,
        ConfirmDialog: true,
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot /><slot name="footer" /></div>',
        },
        Select: {
          props: ['modelValue', 'options'],
          emits: ['update:modelValue', 'change'],
          methods: {
            emitValue(event: Event) {
              const value = (event.target as HTMLSelectElement).value
              this.$emit('update:modelValue', value)
              this.$emit('change', value)
            },
          },
          template: `
            <select :value="modelValue ?? ''" @change="emitValue">
              <option v-for="option in options" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          `,
        },
        PlatformIcon: true,
        GroupCapacityBadge: true,
        GroupRateMultipliersModal: true,
        GroupRPMOverridesModal: true,
        Icon: IconStub,
      },
    },
  })
  mountedWrappers.push(wrapper)
  await flushPromises()
  await nextTick()
  return wrapper
}

const mountKeysView = async () => {
  const wrapper = mount(KeysView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="actions" /><slot name="table" /><slot name="pagination" /></div>',
        },
        DataTable: {
          props: ['columns', 'data'],
          template: `
            <div>
              <div v-for="row in data" :key="row.id">
                <slot name="cell-group" :row="row" />
                <slot name="cell-actions" :row="row" />
              </div>
            </div>
          `,
        },
        Pagination: true,
        EmptyState: true,
        ConfirmDialog: true,
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot /><slot name="footer" /></div>',
        },
        Select: {
          props: ['modelValue', 'options'],
          emits: ['update:modelValue'],
          methods: {
            emitValue(event: Event) {
              this.$emit('update:modelValue', Number((event.target as HTMLSelectElement).value))
            },
          },
          template: `
            <select
              :value="modelValue ?? ''"
              @change="emitValue"
            >
              <option v-for="option in options" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          `,
        },
        SearchInput: true,
        UseKeyModal: true,
        EndpointPopover: true,
        GroupBadge: true,
        GroupOptionItem: true,
        Icon: IconStub,
      },
    },
    attachTo: document.body,
  })
  mountedWrappers.push(wrapper)
  await flushPromises()
  await nextTick()
  return wrapper
}

const mountAllowedGroupsModal = async (user = createUser({ allowed_groups: [] })) => {
  authState.user = {
    id: 1,
    email: 'root@example.com',
    username: 'root',
    role: 'super_admin',
  }
  const wrapper = mount(UserAllowedGroupsModal, {
    props: {
      show: false,
      user,
    },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot /><slot name="footer" /></div>',
        },
        PlatformIcon: true,
      },
    },
  })
  mountedWrappers.push(wrapper)
  await wrapper.setProps({ show: true })
  await flushPromises()
  await nextTick()
  return wrapper
}

describe('group authorization frontend behavior', () => {
  afterEach(() => {
    for (const wrapper of mountedWrappers.splice(0)) {
      wrapper.unmount()
    }
    document.body.innerHTML = ''
  })

  beforeEach(() => {
    document.body.innerHTML = ''
    localStorage.clear()
    authState.user = { id: 1, email: 'admin@example.com', username: 'admin', role: 'admin' }

    listUsers.mockReset().mockResolvedValue({
      items: [createUser()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    getAllGroups.mockReset().mockResolvedValue([])
    getBatchUsersUsage.mockReset().mockResolvedValue({ stats: {} })
    listEnabledDefinitions.mockReset().mockResolvedValue([])
    getBatchUserAttributes.mockReset().mockResolvedValue({ attributes: {} })

    listGroups.mockReset().mockResolvedValue({
      items: [createGroup()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    getGroupUsageSummary.mockReset().mockResolvedValue([])
    getGroupCapacitySummary.mockReset().mockResolvedValue([])
    createGroupRequest.mockReset().mockResolvedValue(createGroup())
    updateGroupRequest.mockReset().mockResolvedValue(createGroup())

    listKeys.mockReset().mockResolvedValue({
      items: [createKey()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    getDashboardApiKeysUsage.mockReset().mockResolvedValue({ stats: {} })
    getAvailableGroups.mockReset().mockResolvedValue([createGroup(), createGroup({ id: 11, name: 'Other' })])
    getUserGroupRates.mockReset().mockResolvedValue({})
    getPublicSettings.mockReset().mockResolvedValue(publicSettings)
    updateKey.mockReset().mockResolvedValue(createKey({ name: 'Updated' }))
    updateUser.mockReset().mockResolvedValue(createUser())
  })

  it('shows user level column and hides allowed-groups action for non-super admins', async () => {
    const wrapper = await mountUsersView()

    expect(wrapper.get('[data-test="columns"]').text().split(',')).toContain('level')
    expect(wrapper.text()).toContain('3')

    await wrapper.get('.action-menu-trigger').trigger('click')
    await nextTick()

    expect(document.body.textContent).not.toContain('admin.users.groups')
  })

  it('shows allowed-groups action for super admins', async () => {
    authState.user = {
      id: 1,
      email: 'root@example.com',
      username: 'root',
      role: 'super_admin',
    }
    const wrapper = await mountUsersView()

    await wrapper.get('.action-menu-trigger').trigger('click')
    await nextTick()

    expect(document.body.textContent).toContain('admin.users.groups')
  })

  it('lets super admins grant restricted non-exclusive groups', async () => {
    listGroups.mockResolvedValue({
      items: [
        createGroup({
          id: 10,
          name: 'Restricted',
          is_exclusive: false,
          access_mode: 'restricted',
          min_user_level: 1,
        }),
      ],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = await mountAllowedGroupsModal()

    const checkbox = wrapper.get('input[type="checkbox"]')
    await checkbox.setValue(true)
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(updateUser).toHaveBeenCalledWith(
      42,
      expect.objectContaining({ allowed_groups: [10] }),
    )
  })

  it('shows restricted groups for explicitly assigned users even below level', async () => {
    localStorage.setItem('user-hidden-columns', JSON.stringify(['notes', 'subscriptions', 'usage', 'concurrency']))
    listUsers.mockResolvedValue({
      items: [
        createUser({ id: 1, level: 5, allowed_groups: [10] }),
        createUser({ id: 2, level: 4, allowed_groups: [10] }),
        createUser({ id: 3, level: 5, allowed_groups: [] }),
      ],
      total: 3,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    getAllGroups.mockResolvedValue([
      createGroup({ id: 10, name: 'Restricted', access_mode: 'restricted', min_user_level: 5 }),
    ])

    const wrapper = await mountUsersView()

    expect(wrapper.text().match(/admin\.users\.restrictedLabel/g) ?? []).toHaveLength(2)
  })

  it('shows group authorization column with access mode and min user level', async () => {
    const wrapper = await mountGroupsView()

    expect(wrapper.get('[data-test="columns"]').text().split(',')).toContain('access')
    expect(wrapper.text()).toContain('admin.groups.accessModes.restricted')
    expect(wrapper.text()).toContain('5')
  })

  it('lets super admins specify visible users when creating a group', async () => {
    authState.user = {
      id: 1,
      email: 'root@example.com',
      username: 'root',
      role: 'super_admin',
    }
    listUsers.mockResolvedValue({
      items: [
        createUser({ id: 7, email: 'target@example.com' }),
      ],
      total: 1,
      page: 1,
      page_size: 1000,
      pages: 1,
    })
    const wrapper = await mountGroupsView()

    await wrapper.find('[data-tour="groups-create-btn"]').trigger('click')
    await flushPromises()
    await wrapper.find('input[data-tour="group-form-name"]').setValue('VIP')
    await wrapper.get('[data-test="create-visible-users"]').setValue('7')
    await wrapper.find('form#create-group-form').trigger('submit')
    await flushPromises()

    expect(createGroupRequest).toHaveBeenCalledWith(
      expect.objectContaining({ visible_user_ids: [7] }),
    )
  })

  it('lets admins set OpenAI image upstream when creating a group', async () => {
    listGroups.mockResolvedValue({
      items: [createGroup({ platform: 'openai', openai_image_upstream: 'auto' })],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const wrapper = await mountGroupsView()

    await wrapper.find('[data-tour="groups-create-btn"]').trigger('click')
    await flushPromises()
    await wrapper.find('input[data-tour="group-form-name"]').setValue('OpenAI Images')
    const platformSelect = wrapper.findAll('select').find((select) =>
      select.findAll('option').some((option) => option.attributes('value') === 'openai') &&
      select.findAll('option').some((option) => option.attributes('value') === 'gemini') &&
      !select.findAll('option').some((option) => option.attributes('value') === ''),
    )!
    await platformSelect.setValue('openai')
    await flushPromises()
    await nextTick()
    await wrapper.vm.$nextTick()
    const upstreamSelect = wrapper.get('select[data-test="create-openai-image-upstream"]')
    await upstreamSelect.setValue('chatgpt_web_image')
    await wrapper.find('form#create-group-form').trigger('submit')
    await flushPromises()

    expect(createGroupRequest).toHaveBeenCalledWith(
      expect.objectContaining({ openai_image_upstream: 'chatgpt_web_image' }),
    )
  })

  it('does not submit group_id when a non-super admin edits an API key', async () => {
    const wrapper = await mountKeysView()

    await wrapper.findAll('button').find((button) => button.text().includes('common.edit'))!.trigger('click')
    await flushPromises()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateKey).toHaveBeenCalledWith(
      99,
      expect.not.objectContaining({ group_id: expect.anything() }),
    )
  })
})
