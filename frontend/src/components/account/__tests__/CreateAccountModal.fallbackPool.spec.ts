import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'),
  'utf8',
)

describe('CreateAccountModal fallback pool propagation', () => {
  it('preserves the selected role for the shared specialized-create helper', () => {
    const helperStart = source.indexOf('const createAccountAndFinish = async')
    const helperEnd = source.indexOf('// Grok', helperStart)

    expect(helperStart).toBeGreaterThanOrEqual(0)
    expect(helperEnd).toBeGreaterThan(helperStart)
    expect(source.slice(helperStart, helperEnd)).toContain('is_fallback: form.is_fallback')
  })
})
