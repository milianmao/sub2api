<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="space-y-4">
          <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">总邮箱</div>
              <div class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ pagination.total }}</div>
              <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">当前页 {{ mailboxes.length }} 条</div>
            </div>
            <div class="rounded-xl border border-emerald-200 bg-emerald-50 p-4 shadow-sm dark:border-emerald-900/50 dark:bg-emerald-900/20">
              <div class="text-xs font-medium uppercase tracking-wide text-emerald-700 dark:text-emerald-300">成功</div>
              <div class="mt-2 text-2xl font-semibold text-emerald-800 dark:text-emerald-200">{{ pageStats.success }}</div>
              <div class="mt-1 text-xs text-emerald-700/80 dark:text-emerald-300/80">当前页 success</div>
            </div>
            <div class="rounded-xl border border-red-200 bg-red-50 p-4 shadow-sm dark:border-red-900/50 dark:bg-red-900/20">
              <div class="text-xs font-medium uppercase tracking-wide text-red-700 dark:text-red-300">失败</div>
              <div class="mt-2 text-2xl font-semibold text-red-800 dark:text-red-200">{{ pageStats.failed }}</div>
              <div class="mt-1 text-xs text-red-700/80 dark:text-red-300/80">当前页 failed</div>
            </div>
            <div class="rounded-xl border border-amber-200 bg-amber-50 p-4 shadow-sm dark:border-amber-900/50 dark:bg-amber-900/20">
              <div class="text-xs font-medium uppercase tracking-wide text-amber-700 dark:text-amber-300">待取码</div>
              <div class="mt-2 text-2xl font-semibold text-amber-800 dark:text-amber-200">{{ pageStats.pending }}</div>
              <div class="mt-1 text-xs text-amber-700/80 dark:text-amber-300/80">当前页未成功取码</div>
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
                <option value="success">成功</option>
                <option value="failed">失败</option>
              </select>
              <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
                <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
                <span>刷新</span>
              </button>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <button type="button" class="btn btn-secondary" @click="openImportDialog">
                <Icon name="upload" size="sm" />
                <span>导入 JSONL</span>
              </button>
            </div>
          </div>

          <div class="rounded-lg border border-blue-200 bg-blue-50 px-3 py-2 text-sm text-blue-800 dark:border-blue-800/50 dark:bg-blue-900/20 dark:text-blue-200">
            卡密邮箱仅支持单行取码。点击目标行“取码”后，验证码会直接显示在表格中，并可一键复制。
          </div>
          <div
            v-if="copyFeedback"
            class="rounded-lg px-3 py-2 text-sm"
            :class="copyFeedback.type === 'success'
              ? 'border border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/50 dark:bg-emerald-900/20 dark:text-emerald-300'
              : 'border border-red-200 bg-red-50 text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300'"
            role="status"
            aria-live="polite"
          >
            {{ copyFeedback.message }}
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="mailboxes" :loading="loading" row-key="id">
          <template #cell-email="{ value }">
            <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
          </template>
          <template #cell-last_status="{ value }">
            <StatusBadge :status="normalizeStatus(value)" :label="statusLabel(value)" />
          </template>
          <template #cell-last_code="{ row, value }">
            <button
              v-if="value"
              type="button"
              class="code-pill"
              :aria-label="`复制 ${row.email} 的验证码 ${value}`"
              @click="copyCode(value)"
            >
              <code>{{ value }}</code>
              <Icon name="copy" size="xs" />
            </button>
            <span v-else class="text-sm text-gray-400">-</span>
          </template>
          <template #cell-last_fetched_at="{ value }">
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
          <template #cell-updated_at="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-300">{{ formatDate(value) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex flex-wrap items-center gap-1">
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

    <BaseDialog :show="showImport" title="导入卡密邮箱 JSONL" width="wide" @close="closeImportDialog">
      <div class="space-y-4">
        <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
          <div class="font-medium text-gray-900 dark:text-white">格式说明</div>
          <p class="mt-1">每行一个 JSON 对象。导入结果只展示数量和行级错误，不展示 mailbox_url、token、password、access_token 或 refresh_token。</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <label class="btn btn-secondary cursor-pointer">
            <Icon name="upload" size="sm" />
            <span>读取 .jsonl 文件</span>
            <input class="hidden" type="file" accept=".jsonl,application/json,text/plain" @change="handleFileSelected" />
          </label>
          <button type="button" class="btn btn-secondary" :disabled="!importContent" @click="clearImportContent">清空</button>
        </div>
        <textarea
          v-model="importContent"
          class="form-textarea min-h-[220px] w-full font-mono text-sm"
          placeholder='{"email":"user@example.com", ...}'
        />
        <div v-if="importResult" class="max-h-64 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-700">
          <div class="grid grid-cols-2 gap-2 border-b border-gray-200 bg-gray-50 px-3 py-2 text-xs font-semibold text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
            <span class="text-emerald-600">成功 {{ importResult.imported }}</span>
            <span class="text-red-600">失败 {{ importResult.failed }}</span>
          </div>
          <div v-if="importResult.errors?.length" class="divide-y divide-gray-100 dark:divide-dark-700">
            <div v-for="err in importResult.errors" :key="`err-${err.line}-${err.message}`" class="grid grid-cols-[4rem_1fr] gap-2 px-3 py-2 text-sm text-red-600 dark:text-red-300">
              <span>{{ err.line }}</span>
              <span>{{ err.message }}</span>
            </div>
          </div>
          <div v-else class="px-3 py-3 text-sm text-gray-500 dark:text-gray-400">没有导入错误。</div>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-secondary" @click="closeImportDialog">关闭</button>
          <button type="button" class="btn btn-primary" :disabled="!importContent.trim() || importing" @click="handleImport">
            <Icon name="upload" size="sm" :class="importing ? 'animate-pulse' : ''" />
            <span>提交导入</span>
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog :show="showFetchCode" title="卡密邮箱验证码结果" width="wide" @close="closeFetchCodeDialog">
      <div v-if="fetchCodeResult" class="space-y-4">
        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
          <div class="text-sm text-gray-500 dark:text-gray-400">邮箱</div>
          <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ fetchCodeResult.email }}</div>
        </div>
        <div v-if="fetchCodeResult.code" class="rounded-xl border border-emerald-200 bg-emerald-50 p-4 dark:border-emerald-900/50 dark:bg-emerald-900/20">
          <div class="text-sm text-emerald-700 dark:text-emerald-300">验证码</div>
          <div class="mt-2 flex items-center justify-between gap-3">
            <button type="button" class="code-pill code-pill-large" @click="copyCode(fetchCodeResult.code)">
              <code>{{ fetchCodeResult.code }}</code>
              <Icon name="copy" size="sm" />
            </button>
          </div>
        </div>
        <div v-if="fetchCodeResult.status !== 'success' || !fetchCodeResult.code" class="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300">
          {{ fetchResultMessage(fetchCodeResult) }}
        </div>
        <div
          v-if="copyFeedback"
          class="rounded-xl p-3 text-sm"
          :class="copyFeedback.type === 'success'
            ? 'border border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/50 dark:bg-emerald-900/20 dark:text-emerald-300'
            : 'border border-red-200 bg-red-50 text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300'"
          role="status"
          aria-live="polite"
        >
          {{ copyFeedback.message }}
        </div>
        <dl class="grid gap-3 sm:grid-cols-2">
          <div class="result-field"><dt>状态</dt><dd>{{ statusLabel(fetchCodeResult.status) }}</dd></div>
          <div class="result-field"><dt>取码时间</dt><dd>{{ formatDate(fetchCodeResult.fetched_at) }}</dd></div>
        </dl>
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
      title="删除卡密邮箱"
      :message="deleteDialog.mailbox ? `确定删除 ${deleteDialog.mailbox.email}？此操作不可恢复。` : ''"
      confirm-text="删除"
      cancel-text="取消"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="closeDeleteDialog"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type {
  CardMailboxFetchCodeResult,
  CardMailboxFetchStatus,
  CardMailboxImportResult,
  CardMailboxListItem,
  CardMailboxListParams
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
  { key: 'email', label: '邮箱' },
  { key: 'last_status', label: '状态' },
  { key: 'last_code', label: '验证码' },
  { key: 'last_fetched_at', label: '最后取码' },
  { key: 'last_error', label: '错误' },
  { key: 'created_at', label: '创建时间' },
  { key: 'updated_at', label: '更新时间' },
  { key: 'actions', label: '操作', width: '160px' }
]

const mailboxes = ref<CardMailboxListItem[]>([])
const loading = ref(false)
const importing = ref(false)
const deleting = ref(false)
const fetchingCodeId = ref<number | null>(null)
const searchTimer = ref<ReturnType<typeof setTimeout> | null>(null)
const copyFeedbackTimer = ref<ReturnType<typeof setTimeout> | null>(null)
const copyFeedback = ref<{ type: 'success' | 'error'; message: string } | null>(null)

const filters = reactive<CardMailboxListParams>({
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
const importContent = ref('')
const importResult = ref<CardMailboxImportResult | null>(null)
const showFetchCode = ref(false)
const fetchCodeResult = ref<CardMailboxFetchCodeResult | null>(null)
const deleteDialog = reactive<{ show: boolean; mailbox: CardMailboxListItem | null }>({
  show: false,
  mailbox: null
})

const pageStats = computed(() => {
  return mailboxes.value.reduce(
    (stats, mailbox) => {
      if (mailbox.last_status === 'success') stats.success += 1
      else if (mailbox.last_status === 'failed') stats.failed += 1
      else stats.pending += 1
      return stats
    },
    { success: 0, failed: 0, pending: 0 }
  )
})

function buildListParams(): CardMailboxListParams {
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
    const result = await adminAPI.cardMailboxes.list(buildListParams())
    mailboxes.value = result.items || []
    pagination.total = result.total || 0
    pagination.page = result.page || pagination.page
    pagination.page_size = result.page_size || pagination.page_size
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

function isRowBusy(id: number) {
  return fetchingCodeId.value === id || deleting.value
}

async function handleFetchCode(row: CardMailboxListItem) {
  if (fetchingCodeId.value !== null) return

  fetchingCodeId.value = row.id
  fetchCodeResult.value = null
  showFetchCode.value = true
  try {
    const result = await adminAPI.cardMailboxes.fetchCode(row.id)
    fetchCodeResult.value = result
    updateRowFromFetchResult(row.id, result)
  } catch (error) {
    fetchCodeResult.value = {
      email: row.email,
      code: '',
      status: 'failed',
      fetched_at: new Date().toISOString(),
      source: '',
      subject: '',
      from: '',
      received_at: '',
      snippet: ''
    }
    updateRowFromFetchResult(row.id, fetchCodeResult.value, error instanceof Error ? error.message : '获取验证码失败')
  } finally {
    fetchingCodeId.value = null
  }
}

function updateRowFromFetchResult(id: number, result: CardMailboxFetchCodeResult, errorMessage?: string) {
  const nextError = resolveFetchError(result, errorMessage)
  mailboxes.value = mailboxes.value.map(mailbox => {
    if (mailbox.id !== id) return mailbox
    return {
      ...mailbox,
      last_code: result.status === 'success' && result.code ? result.code : '',
      last_status: result.status,
      last_error: nextError,
      last_fetched_at: result.fetched_at || mailbox.last_fetched_at,
      updated_at: result.fetched_at || mailbox.updated_at
    }
  })
}

function resolveFetchError(result: CardMailboxFetchCodeResult, errorMessage?: string) {
  if (errorMessage) return errorMessage
  if (result.status !== 'success') return statusLabel(result.status)
  if (!result.code) return '未获取到验证码'
  return null
}

function fetchResultMessage(result: CardMailboxFetchCodeResult) {
  return resolveFetchError(result) || statusLabel(result.status)
}

function openDeleteDialog(row: CardMailboxListItem) {
  deleteDialog.mailbox = row
  deleteDialog.show = true
}

function closeDeleteDialog() {
  deleteDialog.show = false
  deleteDialog.mailbox = null
}

async function confirmDelete() {
  if (!deleteDialog.mailbox) return
  deleting.value = true
  try {
    await adminAPI.cardMailboxes.delete(deleteDialog.mailbox.id)
    closeDeleteDialog()
    await load()
  } finally {
    deleting.value = false
  }
}

function openImportDialog() {
  showImport.value = true
}

function closeImportDialog() {
  showImport.value = false
}

function clearImportContent() {
  importContent.value = ''
  importResult.value = null
}

async function handleImport() {
  const content = importContent.value.trim()
  if (!content) return
  importing.value = true
  try {
    importResult.value = await adminAPI.cardMailboxes.importJSONL({ content })
    await load()
  } finally {
    importing.value = false
  }
}

async function handleFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  importContent.value = await file.text()
  importResult.value = null
  input.value = ''
}

function closeFetchCodeDialog() {
  showFetchCode.value = false
  fetchCodeResult.value = null
}

async function copyCode(code: string) {
  if (!code) return
  try {
    if (!navigator.clipboard?.writeText) {
      throw new Error('当前浏览器不支持剪贴板写入')
    }
    await navigator.clipboard.writeText(code)
    showCopyFeedback('success', '验证码已复制')
  } catch (error) {
    showCopyFeedback('error', error instanceof Error ? error.message : '复制失败，请手动复制')
  }
}

function showCopyFeedback(type: 'success' | 'error', message: string) {
  copyFeedback.value = { type, message }
  if (copyFeedbackTimer.value) clearTimeout(copyFeedbackTimer.value)
  copyFeedbackTimer.value = setTimeout(() => {
    copyFeedback.value = null
    copyFeedbackTimer.value = null
  }, 2200)
}

function normalizeStatus(status?: CardMailboxFetchStatus) {
  if (status === 'success') return 'success'
  if (status === 'failed') return 'error'
  return 'warning'
}

function statusLabel(status?: CardMailboxFetchStatus) {
  const labels: Record<string, string> = {
    success: '成功',
    failed: '失败'
  }
  return labels[status || ''] || status || '未取码'
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

.code-pill {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  border-radius: 9999px;
  border: 1px solid rgb(16 185 129 / 0.35);
  background-color: rgb(236 253 245);
  padding: 0.25rem 0.625rem;
  color: rgb(6 95 70);
  transition: background-color 150ms ease, border-color 150ms ease, color 150ms ease;
}

.code-pill:hover {
  border-color: rgb(5 150 105 / 0.6);
  background-color: rgb(209 250 229);
  color: rgb(4 120 87);
}

.code-pill code {
  font-weight: 700;
  letter-spacing: 0.08em;
}

.code-pill-large {
  border-radius: 0.75rem;
  padding: 0.75rem 1rem;
  font-size: 1.875rem;
  line-height: 2.25rem;
}

.dark .code-pill {
  border-color: rgb(16 185 129 / 0.4);
  background-color: rgb(6 78 59 / 0.35);
  color: rgb(167 243 208);
}

.dark .code-pill:hover {
  border-color: rgb(52 211 153 / 0.7);
  background-color: rgb(6 95 70 / 0.45);
  color: rgb(209 250 229);
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
