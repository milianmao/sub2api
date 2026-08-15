import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('GroupsView fallback pool metadata', () => {
  it('renders read-only pool metadata for account search results and routing chips', () => {
    const source = readFileSync('src/views/admin/GroupsView.vue', 'utf8')

    expect(source).toContain("account.is_fallback ? t('admin.accounts.fallbackPool') : t('admin.accounts.primaryPool')")
    expect(source).toContain('is_fallback: account.is_fallback === true')
    expect(source).toContain('accounts.push({ id: account.id, name: account.name, is_fallback: account.is_fallback === true });')
  })
})
