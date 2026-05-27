import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const currentDir = dirname(fileURLToPath(import.meta.url))
const keysViewPath = resolve(currentDir, '../KeysView.vue')
const source = readFileSync(keysViewPath, 'utf8')

describe('KeysView API Key 分组修改', () => {
  it('编辑弹窗对普通用户显示分组选择器', () => {
    expect(source).toContain('<div>\n          <GroupSelector')
  })

  it('分页列表分组列点击时打开分组选择器', () => {
    expect(source).toContain('@click="openGroupSelector(row)"')
  })

  it('编辑提交同时发送主分组和授权分组', () => {
    expect(source).toContain('group_id: groupIds[0] ?? null')
    expect(source).toContain('group_ids: groupIds')
  })
})
