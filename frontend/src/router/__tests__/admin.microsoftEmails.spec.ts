import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const routerPath = resolve(dirname(fileURLToPath(import.meta.url)), '../index.ts')
const routerSource = readFileSync(routerPath, 'utf8')

describe('admin microsoft email route', () => {
  it('registers a protected admin route that lazy-loads the microsoft emails view', () => {
    const routeBlockMatch = routerSource.match(/\{\s*path: '\/admin\/microsoft-emails',[\s\S]*?\n  \}/)

    expect(routeBlockMatch).not.toBeNull()
    const routeBlock = routeBlockMatch?.[0] ?? ''

    expect(routeBlock).toContain("name: 'AdminMicrosoftEmails'")
    expect(routeBlock).toContain("component: () => import('@/views/admin/MicrosoftEmailsView.vue')")
    expect(routeBlock).toContain('requiresAuth: true')
    expect(routeBlock).toContain('requiresAdmin: true')
  })
})
