<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="space-y-4">
          <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">总账号</div>
              <div class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ pagination.total }}</div>
              <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">当前页 {{ accounts.length }} 条</div>
            </div>
            <div class="rounded-xl border border-emerald-200 bg-emerald-50 p-4 shadow-sm dark:border-emerald-900/50 dark:bg-emerald-900/20">
              <div class="text-xs font-medium uppercase tracking-wide text-emerald-700 dark:text-emerald-300">正常</div>
              <div class="mt-2 text-2xl font-semibold text-emerald-800 dark:text-emerald-200">{{ pageStats.active }}</div>
              <div class="mt-1 text-xs text-emerald-700/80 dark:text-emerald-300/80">当前页 active</div>
            </div>
            <div class="rounded-xl border border-red-200 bg-red-50 p-4 shadow-sm dark:border-red-900/50 dark:bg-red-900/20">
              <div class="text-xs font-medium uppercase tracking-wide text-red-700 dark:text-red-300">异常</div>
              <div class="mt-2 text-2xl font-semibold text-red-800 dark:text-red-200">{{ pageStats.error }}</div>
              <div class="mt-1 text-xs text-red-700/80 dark:text-red-300/80">当前页 error</div>
            </div>
            <div class="rounded-xl border border-amber-200 bg-amber-50 p-4 shadow-sm dark:border-amber-900/50 dark:bg-amber-900/20">
              <div class="text-xs font-medium uppercase tracking-wide text-amber-700 dark:text-amber-300">待检查</div>
              <div class="mt-2 text-2xl font-semibold text-amber-800 dark:text-amber-200">{{ pageStats.unchecked }}</div>
              <div class="mt-1 text-xs text-amber-700/80 dark:text-amber-300/80">当前页 unchecked/inactive</div>
            </div>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-gray-200 bg-white p-3 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="flex flex-1 flex-wrap items-center gap-2">
              <div class="relative min-w-[220px] flex-1 sm:flex-none">
                <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input
                  v-model="filters.search"
                  type="search"
                  class="form-input w-full pl-9"
                  placeholder="搜索邮箱"
                  @input="handleSearchInput"
                />
              </div>
              <select v-model="filters.status" class="form-select min-w-[150px]" @change="reloadFirstPage">
                <option value="">全部状态</option>
                <option value="active">正常</option>
                <option value="invalid">失效</option>
                <option value="error">异常</option>
                <option value="unchecked">待检查</option>
                <option value="inactive">停用</option>
              </select>
              <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
                <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
                <span>刷新</span>
              </button>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <button type="button" class="btn btn-secondary" :disabled="importing" @click="openImportFilePicker">
                <Icon name="upload" size="sm" :class="importing ? 'animate-pulse' : ''" />
                <span>{{ importing ? '导入中...' : '导入 TXT' }}</span>
              </button>
              <input ref="importFileInput" class="hidden" type="file" accept=".txt,text/plain" @change="handleFileSelected" />
              <button type="button" class="btn btn-secondary" :disabled="!selectedIds.length || batchChecking" @click="handleBatchCheck">
                <Icon name="sync" size="sm" :class="batchChecking ? 'animate-spin' : ''" />
                <span>批量健康检查</span>
                <span v-if="selectedIds.length" class="rounded-full bg-primary-100 px-2 py-0.5 text-xs text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">{{ selectedIds.length }}</span>
              </button>
              <button type="button" class="btn btn-danger" :disabled="!selectedIds.length || deleting" @click="openBatchDeleteDialog">
                <Icon name="trash" size="sm" />
                <span>批量删除</span>
              </button>
            </div>
          </div>

          <div class="rounded-lg border border-blue-200 bg-blue-50 px-3 py-2 text-sm text-blue-800 dark:border-blue-800/50 dark:bg-blue-900/20 dark:text-blue-200">
            验证码获取仅支持单个账号。请在目标行点击“取码”，不会提供批量取码入口。
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="accounts" :loading="loading" row-key="id">
          <template #header-select>
            <input
              type="checkbox"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="allVisibleSelected"
              :disabled="!accounts.length"
              @change="toggleSelectAllVisible"
            />
          </template>
          <template #cell-select="{ row }">
            <input
              type="checkbox"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="selectedIds.includes(row.id)"
              @change="toggleSelection(row.id)"
            />
          </template>
          <template #cell-email="{ value }">
            <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
          </template>
          <template #cell-status="{ value }">
            <StatusBadge :status="normalizeStatus(value)" :label="statusLabel(value)" />
          </template>
          <template #cell-client_id="{ value }">
            <code class="rounded bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ maskClientId(value) }}</code>
          </template>
          <template #cell-last_check_at="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-300">{{ formatDate(value) }}</span>
          </template>
          <template #cell-last_fetch_at="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-300">{{ formatDate(value) }}</span>
          </template>
          <template #cell-last_error="{ value }">
            <span class="line-clamp-2 max-w-xs text-sm" :class="value ? 'text-red-600 dark:text-red-300' : 'text-gray-400'">
              {{ value || '-' }}
            </span>
          </template>
          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-300">{{ formatDate(value) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex flex-wrap items-center gap-1">
              <button type="button" class="row-action" :disabled="isRowBusy(row.id)" @click="handleCheck(row)">
                <Icon name="sync" size="sm" :class="checkingIds.has(row.id) ? 'animate-spin' : ''" />
                <span>检查</span>
              </button>
              <button type="button" class="row-action text-blue-600 hover:text-blue-700 dark:text-blue-300" :disabled="isRowBusy(row.id)" @click="handleFetchCode(row)">
                <Icon name="mail" size="sm" :class="fetchingCodeId === row.id ? 'animate-pulse' : ''" />
                <span>取码</span>
              </button>
              <button type="button" class="row-action text-red-600 hover:text-red-700 dark:text-red-300" :disabled="isRowBusy(row.id)" @click="openDeleteDialog(row)">
                <Icon name="trash" size="sm" />
                <span>删除</span>
              </button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <BaseDialog :show="showImport" title="导入结果" width="wide" @close="closeImportDialog">
      <div class="space-y-4">
        <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
          <div class="font-medium text-gray-900 dark:text-white">格式说明</div>
          <p class="mt-1">每行一个账号：email----password----client_id----refresh_token。导入结果只展示行号、邮箱和错误，不展示密码或 refresh_token。</p>
        </div>
        <div v-if="importResult" class="max-h-64 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-700">
          <div class="grid grid-cols-4 gap-2 border-b border-gray-200 bg-gray-50 px-3 py-2 text-xs font-semibold text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
            <span>总数 {{ importResult.total }}</span>
            <span class="text-emerald-600">新增 {{ importResult.created }}</span>
            <span class="text-blue-600">更新 {{ importResult.updated }}</span>
            <span class="text-red-600">失败 {{ importResult.failed }}</span>
          </div>
          <div class="divide-y divide-gray-100 dark:divide-dark-700">
            <div v-for="item in importResult.items" :key="`ok-${item.line}-${item.email}`" class="grid grid-cols-[4rem_1fr_6rem] gap-2 px-3 py-2 text-sm">
              <span class="text-gray-500">{{ item.line }}</span>
              <span class="truncate">{{ item.email }}</span>
              <span class="text-emerald-600">{{ item.action }}</span>
            </div>
            <div v-for="err in importResult.errors" :key="`err-${err.line}-${err.email || err.error}`" class="grid grid-cols-[4rem_1fr_1.5fr] gap-2 px-3 py-2 text-sm text-red-600 dark:text-red-300">
              <span>{{ err.line }}</span>
              <span class="truncate">{{ err.email || '-' }}</span>
              <span>{{ err.error }}</span>
            </div>
          </div>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-secondary" @click="closeImportDialog">关闭</button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog :show="showFetchCode" title="单账号验证码结果" width="wide" @close="closeFetchCodeDialog">
      <div v-if="fetchCodeResult" class="space-y-4">
        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
          <div class="text-sm text-gray-500 dark:text-gray-400">邮箱</div>
          <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ fetchCodeResult.email }}</div>
        </div>
        <div v-if="fetchCodeResult.code" class="rounded-xl border border-emerald-200 bg-emerald-50 p-4 dark:border-emerald-900/50 dark:bg-emerald-900/20">
          <div class="text-sm text-emerald-700 dark:text-emerald-300">验证码</div>
          <div class="mt-2 flex items-center justify-between gap-3">
            <code class="text-3xl font-bold tracking-widest text-emerald-800 dark:text-emerald-100">{{ fetchCodeResult.code }}</code>
            <button type="button" class="btn btn-secondary" @click="copyCode(fetchCodeResult.code)">
              <Icon name="copy" size="sm" />
              <span>复制</span>
            </button>
          </div>
        </div>
        <div v-if="fetchCodeResult.error" class="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300">
          {{ fetchCodeResult.error }}
        </div>
        <dl class="grid gap-3 sm:grid-cols-2">
          <div class="result-field"><dt>主题</dt><dd>{{ fetchCodeResult.subject || '-' }}</dd></div>
          <div class="result-field"><dt>发件人</dt><dd>{{ fetchCodeResult.from || '-' }}</dd></div>
          <div class="result-field"><dt>来源</dt><dd>{{ fetchCodeResult.source || '-' }}</dd></div>
          <div class="result-field"><dt>收件时间</dt><dd>{{ formatDate(fetchCodeResult.received_at) }}</dd></div>
        </dl>
        <div class="result-field">
          <dt>摘要</dt>
          <dd class="whitespace-pre-wrap">{{ fetchCodeResult.snippet || '-' }}</dd>
        </div>
      </div>
      <div v-else class="rounded-xl border border-blue-200 bg-blue-50 p-4 text-sm text-blue-800 dark:border-blue-800/50 dark:bg-blue-900/20 dark:text-blue-200">
        正在获取验证码，请稍候...
      </div>
      <template #footer>
        <button type="button" class="btn btn-primary" @click="closeFetchCodeDialog">知道了</button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="deleteDialog.show"
      title="删除微软邮箱"
      :message="deleteDialog.account ? `确定删除 ${deleteDialog.account.email}？此操作不可恢复。` : ''"
      confirm-text="删除"
      cancel-text="取消"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="deleteDialog.show = false"
    />

    <ConfirmDialog
      :show="batchDeleteDialog"
      title="批量删除微软邮箱"
      :message="`确定删除已选择的 ${selectedIds.length} 个账号？此操作不可恢复。`"
      confirm-text="批量删除"
      cancel-text="取消"
      :danger="true"
      @confirm="confirmBatchDelete"
      @cancel="batchDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type {
  MicrosoftEmailBatchCheckResult,
  MicrosoftEmailFetchCodeResult,
  MicrosoftEmailImportResult,
  MicrosoftEmailListItem,
  MicrosoftEmailListParams,
  MicrosoftEmailStatus
} from '@/api/admin'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import Icon from '@/components/icons/Icon.vue'

interface Column {
  key: string
  label: string
  width?: string
}

const columns: Column[] = [
  { key: 'select', label: '', width: '48px' },
  { key: 'email', label: '邮箱' },
  { key: 'status', label: '状态' },
  { key: 'client_id', label: 'Client ID' },
  { key: 'last_check_at', label: '最后检查' },
  { key: 'last_fetch_at', label: '最后取码' },
  { key: 'last_error', label: '错误' },
  { key: 'created_at', label: '创建时间' },
  { key: 'actions', label: '操作', width: '220px' }
]

const accounts = ref<MicrosoftEmailListItem[]>([])
const loading = ref(false)
const importing = ref(false)
const batchChecking = ref(false)
const deleting = ref(false)
const fetchingCodeId = ref<number | null>(null)
const checkingIds = ref(new Set<number>())
const selectedIds = ref<number[]>([])
const searchTimer = ref<ReturnType<typeof setTimeout> | null>(null)

const filters = reactive<MicrosoftEmailListParams>({
  page: 1,
  page_size: 20,
  search: '',
  status: ''
})

const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0
})

const showImport = ref(false)
const importFileInput = ref<HTMLInputElement | null>(null)
const importResult = ref<MicrosoftEmailImportResult | null>(null)
const showFetchCode = ref(false)
const fetchCodeResult = ref<MicrosoftEmailFetchCodeResult | null>(null)
const batchDeleteDialog = ref(false)
const deleteDialog = reactive<{ show: boolean; account: MicrosoftEmailListItem | null }>({
  show: false,
  account: null
})

const pageStats = computed(() => {
  return accounts.value.reduce(
    (stats, account) => {
      if (account.status === 'active') stats.active += 1
      else if (account.status === 'error') stats.error += 1
      else stats.unchecked += 1
      return stats
    },
    { active: 0, error: 0, unchecked: 0 }
  )
})

const allVisibleSelected = computed(() => {
  return accounts.value.length > 0 && accounts.value.every(account => selectedIds.value.includes(account.id))
})

function buildListParams(): MicrosoftEmailListParams {
  return {
    page: pagination.page,
    page_size: pagination.page_size,
    search: filters.search?.trim() || undefined,
    status: filters.status || undefined
  }
}

async function load() {
  loading.value = true
  try {
    const result = await adminAPI.microsoftEmails.list(buildListParams())
    accounts.value = result.items || []
    pagination.total = result.total || 0
    pagination.page = result.page || pagination.page
    pagination.page_size = result.page_size || pagination.page_size
    selectedIds.value = selectedIds.value.filter(id => accounts.value.some(account => account.id === id))
  } finally {
    loading.value = false
  }
}

function reloadFirstPage() {
  pagination.page = 1
  void load()
}

function handleSearchInput() {
  if (searchTimer.value) clearTimeout(searchTimer.value)
  searchTimer.value = setTimeout(() => {
    reloadFirstPage()
  }, 300)
}

function handlePageChange(page: number) {
  pagination.page = page
  void load()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  void load()
}

function toggleSelection(id: number) {
  selectedIds.value = selectedIds.value.includes(id)
    ? selectedIds.value.filter(selectedId => selectedId !== id)
    : [...selectedIds.value, id]
}

function toggleSelectAllVisible(event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  const visibleIds = accounts.value.map(account => account.id)
  if (checked) {
    selectedIds.value = Array.from(new Set([...selectedIds.value, ...visibleIds]))
    return
  }
  selectedIds.value = selectedIds.value.filter(id => !visibleIds.includes(id))
}

function isRowBusy(id: number) {
  return checkingIds.value.has(id) || fetchingCodeId.value !== null || deleting.value
}

async function handleCheck(row: MicrosoftEmailListItem) {
  checkingIds.value = new Set([...checkingIds.value, row.id])
  try {
    await adminAPI.microsoftEmails.check(row.id)
    await load()
  } finally {
    const next = new Set(checkingIds.value)
    next.delete(row.id)
    checkingIds.value = next
  }
}

async function handleBatchCheck() {
  if (!selectedIds.value.length) return
  batchChecking.value = true
  try {
    const result: MicrosoftEmailBatchCheckResult = await adminAPI.microsoftEmails.batchCheck(selectedIds.value)
    if (result.items?.length) {
      const byId = new Map(result.items.map(item => [item.id, item]))
      accounts.value = accounts.value.map(account => {
        const checked = byId.get(account.id)
        if (!checked) return account
        return {
          ...account,
          status: checked.status,
          last_check_at: checked.checked_at ?? account.last_check_at,
          last_error: checked.last_error ?? account.last_error
        }
      })
    } else {
      await load()
    }
  } finally {
    batchChecking.value = false
  }
}

async function handleFetchCode(row: MicrosoftEmailListItem) {
  if (fetchingCodeId.value !== null) return

  fetchingCodeId.value = row.id
  fetchCodeResult.value = null
  showFetchCode.value = true
  try {
    fetchCodeResult.value = await adminAPI.microsoftEmails.fetchCode(row.id)
    await load()
  } catch (error) {
    fetchCodeResult.value = {
      email: row.email,
      code: '',
      source: '',
      subject: '',
      from: '',
      received_at: '',
      snippet: '',
      error: error instanceof Error ? error.message : '获取验证码失败'
    }
  } finally {
    fetchingCodeId.value = null
  }
}

function openDeleteDialog(row: MicrosoftEmailListItem) {
  deleteDialog.account = row
  deleteDialog.show = true
}

async function confirmDelete() {
  if (!deleteDialog.account) return
  deleting.value = true
  try {
    await adminAPI.microsoftEmails.delete(deleteDialog.account.id)
    selectedIds.value = selectedIds.value.filter(id => id !== deleteDialog.account?.id)
    deleteDialog.show = false
    deleteDialog.account = null
    await load()
  } finally {
    deleting.value = false
  }
}

function openBatchDeleteDialog() {
  if (!selectedIds.value.length) return
  batchDeleteDialog.value = true
}

async function confirmBatchDelete() {
  if (!selectedIds.value.length) return
  deleting.value = true
  try {
    await adminAPI.microsoftEmails.batchDelete(selectedIds.value)
    selectedIds.value = []
    batchDeleteDialog.value = false
    await load()
  } finally {
    deleting.value = false
  }
}

function openImportFilePicker() {
  importFileInput.value?.click()
}

function closeImportDialog() {
  showImport.value = false
}

async function importTXTContent(content: string) {
  const normalizedContent = content.trim()
  if (!normalizedContent || importing.value) return
  importing.value = true
  try {
    importResult.value = await adminAPI.microsoftEmails.importTXT({ content: normalizedContent })
    showImport.value = true
    await load()
  } finally {
    importing.value = false
  }
}

async function handleFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  await importTXTContent(await file.text())
}

function closeFetchCodeDialog() {
  showFetchCode.value = false
  fetchCodeResult.value = null
}

async function copyCode(code: string) {
  if (!code) return
  await navigator.clipboard?.writeText(code)
}

function maskClientId(value?: string) {
  if (!value) return '-'
  if (value.length <= 8) return `${value.slice(0, 2)}***${value.slice(-2)}`
  return `${value.slice(0, 4)}****${value.slice(-4)}`
}

function normalizeStatus(status: MicrosoftEmailStatus) {
  if (status === 'active') return 'active'
  if (status === 'error') return 'error'
  if (status === 'invalid') return 'error'
  if (status === 'inactive') return 'inactive'
  return 'warning'
}

function statusLabel(status: MicrosoftEmailStatus) {
  const labels: Record<string, string> = {
    active: '正常',
    invalid: '失效',
    error: '异常',
    unchecked: '待检查',
    inactive: '停用'
  }
  return labels[status] || status || '未知'
}

function formatDate(value?: string | null) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

onMounted(() => {
  void load()
})
</script>

<style scoped>
.row-action {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  border-radius: 0.5rem;
  padding: 0.375rem 0.5rem;
  font-size: 0.75rem;
  line-height: 1rem;
  color: rgb(75 85 99);
  transition: background-color 150ms ease, color 150ms ease;
}

.row-action:hover:not(:disabled) {
  background-color: rgb(243 244 246);
  color: rgb(17 24 39);
}

.row-action:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.dark .row-action {
  color: rgb(209 213 219);
}

.dark .row-action:hover:not(:disabled) {
  background-color: rgb(31 41 55);
  color: rgb(255 255 255);
}

.result-field {
  border-radius: 0.75rem;
  border: 1px solid rgb(229 231 235);
  padding: 0.75rem;
}

.dark .result-field {
  border-color: rgb(55 65 81);
}

.result-field dt {
  font-size: 0.75rem;
  line-height: 1rem;
  color: rgb(107 114 128);
}

.result-field dd {
  margin-top: 0.25rem;
  word-break: break-word;
  color: rgb(17 24 39);
}

.dark .result-field dd {
  color: rgb(243 244 246);
}
</style>
