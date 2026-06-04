import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const routerPath = resolve(dirname(fileURLToPath(import.meta.url)), '../index.ts')
const routerSource = readFileSync(routerPath, 'utf8')

describe('admin chatgpt plus checkout route', () => {
  it('registers a protected admin route that lazy-loads the checkout view', () => {
    const routeBlockMatch = routerSource.match(/\{\s*path: '\/admin\/chatgpt-plus-checkout',[\s\S]*?\n  \}/)

    expect(routeBlockMatch).not.toBeNull()
    const routeBlock = routeBlockMatch?.[0] ?? ''

    expect(routeBlock).toContain("name: 'AdminChatGPTPlusCheckout'")
    expect(routeBlock).toContain("component: () => import('@/views/admin/ChatGPTPlusCheckoutView.vue')")
    expect(routeBlock).toContain('requiresAuth: true')
    expect(routeBlock).toContain('requiresAdmin: true')
  })
})
