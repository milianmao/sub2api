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

  it('分页列表分组下拉以授权分组多选方式切换', () => {
    expect(source).toContain('@click="togglePendingAuthorizedGroup(option.value)"')
    expect(source).toContain('isPendingAuthorizedGroupSelected(option.value)')
    expect(source).toContain('await commitPendingGroupSelection()')
    expect(source).toContain('group_id: pendingGroupIds.value[0] ?? null')
    expect(source).toContain('group_ids: pendingGroupIds.value')
  })

  it('编辑提交同时发送主分组和授权分组', () => {
    expect(source).toContain('group_id: groupIds[0] ?? null')
    expect(source).toContain('group_ids: groupIds')
  })
})
