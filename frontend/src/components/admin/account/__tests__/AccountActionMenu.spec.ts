import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AccountActionMenu from '../AccountActionMenu.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const baseAccount = {
  id: 42,
  name: 'OpenAI OAuth',
  platform: 'openai',
  type: 'oauth',
  status: 'active'
}

function mountMenu(account: Record<string, unknown>) {
  return mount(AccountActionMenu, {
    props: {
      show: true,
      account: account as any,
      position: { top: 10, left: 20 }
    },
    global: {
      stubs: {
        Icon: true,
        Teleport: true
      }
    }
  })
}

describe('AccountActionMenu checkout link action', () => {
  it('shows copy access token action for OAuth accounts and emits the selected account', async () => {
    const wrapper = mountMenu(baseAccount)

    expect(wrapper.text()).toContain('admin.accounts.copyAccessToken')

    const button = wrapper.findAll('button').find(item => item.text().includes('admin.accounts.copyAccessToken'))
    expect(button).toBeTruthy()

    await button!.trigger('click')

    expect(wrapper.emitted('copy-access-token')?.[0]).toEqual([baseAccount])
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('hides copy access token action for non OAuth accounts', () => {
    const wrapper = mountMenu({
      ...baseAccount,
      type: 'apikey'
    })

    expect(wrapper.text()).not.toContain('admin.accounts.copyAccessToken')
  })

  it('shows checkout link action for OpenAI OAuth accounts and emits the selected account', async () => {
    const wrapper = mountMenu(baseAccount)

    expect(wrapper.text()).toContain('admin.accounts.generateCheckoutLink')

    const button = wrapper.findAll('button').find(item => item.text().includes('admin.accounts.generateCheckoutLink'))
    expect(button).toBeTruthy()

    await button!.trigger('click')

    expect(wrapper.emitted('generate-checkout-link')?.[0]).toEqual([baseAccount])
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('hides checkout link action for non OpenAI OAuth accounts', () => {
    const wrapper = mountMenu({
      ...baseAccount,
      platform: 'anthropic'
    })

    expect(wrapper.text()).not.toContain('admin.accounts.generateCheckoutLink')
  })

  it('hides checkout link action for OpenAI non OAuth accounts', () => {
    const wrapper = mountMenu({
      ...baseAccount,
      type: 'apikey'
    })

    expect(wrapper.text()).not.toContain('admin.accounts.generateCheckoutLink')
  })
})
