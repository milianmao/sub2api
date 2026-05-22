<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.chatgptSessionImportTitle')"
    width="wide"
    close-on-click-outside
    @close="handleClose"
  >
    <form id="import-chatgpt-session-form" class="space-y-4" @submit.prevent="handleImport">
      <div class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('admin.accounts.chatgptSessionImportHint') }}
      </div>

      <div class="grid gap-4 md:grid-cols-2">
        <div class="md:col-span-2">
          <label class="input-label">{{ t('admin.accounts.chatgptSessionImportText') }}</label>
          <textarea
            v-model="form.content"
            rows="8"
            class="input"
            :placeholder="t('admin.accounts.chatgptSessionImportPlaceholder')"
          ></textarea>
        </div>

        <div class="md:col-span-2">
          <label class="input-label">{{ t('admin.accounts.chatgptSessionImportFiles') }}</label>
          <div class="rounded-lg border border-dashed border-gray-300 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-800">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div class="min-w-0">
                <div class="truncate text-sm text-gray-700 dark:text-dark-200">
                  {{ selectedFileLabel }}
                </div>
                <div class="text-xs text-gray-500 dark:text-dark-400">JSON (.json)</div>
              </div>
              <button type="button" class="btn btn-secondary shrink-0" @click="openFilePicker">
                {{ t('common.chooseFile') }}
              </button>
            </div>
          </div>
          <input
            ref="fileInput"
            type="file"
            multiple
            class="hidden"
            accept="application/json,.json"
            @change="handleFileChange"
          />
        </div>

        <div class="md:col-span-2">
          <label class="input-label">{{ t('admin.accounts.notes') }}</label>
          <textarea
            v-model="form.notes"
            rows="2"
            class="input"
            :placeholder="t('admin.accounts.notesPlaceholder')"
          ></textarea>
        </div>

        <div class="md:col-span-2">
          <GroupSelector v-model="form.group_ids" :groups="groups" />
        </div>

        <div>
          <label class="input-label">{{ t('admin.accounts.proxy') }}</label>
          <ProxySelector v-model="form.proxy_id" :proxies="proxies" />
        </div>

        <div>
          <label class="input-label">{{ t('admin.accounts.concurrency') }}</label>
          <input v-model.number="form.concurrency" type="number" min="0" class="input" />
        </div>

        <div>
          <label class="input-label">{{ t('admin.accounts.priority') }}</label>
          <input v-model.number="form.priority" type="number" min="0" class="input" />
        </div>

        <div>
          <label class="input-label">{{ t('admin.accounts.billingRateMultiplier') }}</label>
          <input v-model.number="form.rate_multiplier" type="number" min="0" step="0.001" class="input" />
        </div>

        <div>
          <label class="input-label">{{ t('admin.accounts.loadFactor') }}</label>
          <input v-model.number="form.load_factor" type="number" min="1" class="input" />
        </div>

        <div>
          <label class="input-label">{{ t('admin.accounts.columns.expiresAt') }}</label>
          <input v-model="expiresAtInput" type="datetime-local" class="input" />
        </div>

        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input
            v-model="form.auto_pause_on_expired"
            type="checkbox"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
          <span>{{ t('admin.accounts.autoPauseOnExpired') }}</span>
        </label>
      </div>

      <div
        v-if="result"
        class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-700"
      >
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.chatgptSessionImportResult') }}
        </div>
        <div class="text-sm text-gray-700 dark:text-dark-300">
          {{ t('admin.accounts.chatgptSessionImportResultSummary', result) }}
        </div>

        <div v-if="result.errors?.length" class="space-y-2">
          <div class="text-sm font-medium text-red-600 dark:text-red-400">
            {{ t('admin.accounts.chatgptSessionImportErrors') }}
          </div>
          <div class="max-h-48 overflow-auto rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800">
            <div v-for="item in result.errors" :key="`${item.index}-${item.name || ''}`" class="whitespace-pre-wrap">
              {{ item.index }}. {{ item.name || '-' }} - {{ item.message }}
            </div>
          </div>
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" :disabled="importing" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" type="submit" form="import-chatgpt-session-form" :disabled="importing">
          {{ importing ? t('admin.accounts.chatgptSessionImporting') : t('admin.accounts.chatgptSessionImportButton') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/utils/format'
import type { AdminGroup, ChatGPTSessionImportResult, Proxy } from '@/types'

interface Props {
  show: boolean
  groups: AdminGroup[]
  proxies: Proxy[]
}

interface Emits {
  (e: 'close'): void
  (e: 'imported'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const importing = ref(false)
const files = ref<File[]>([])
const fileInput = ref<HTMLInputElement | null>(null)
const result = ref<ChatGPTSessionImportResult | null>(null)

const form = reactive({
  content: '',
  notes: '',
  group_ids: [] as number[],
  proxy_id: null as number | null,
  concurrency: 3,
  priority: 50,
  rate_multiplier: 1,
  load_factor: null as number | null,
  expires_at: null as number | null,
  auto_pause_on_expired: true
})

const expiresAtInput = computed({
  get: () => formatDateTimeLocalInput(form.expires_at),
  set: (value: string) => {
    form.expires_at = parseDateTimeLocalInput(value)
  }
})

const selectedFileLabel = computed(() => {
  if (files.value.length === 0) {
    return t('admin.accounts.chatgptSessionImportSelectFiles')
  }
  return files.value.map(file => file.name).join(', ')
})

watch(
  () => props.show,
  (open) => {
    if (!open) return
    form.content = ''
    form.notes = ''
    form.group_ids = []
    form.proxy_id = null
    form.concurrency = 3
    form.priority = 50
    form.rate_multiplier = 1
    form.load_factor = null
    form.expires_at = null
    form.auto_pause_on_expired = true
    files.value = []
    result.value = null
    if (fileInput.value) {
      fileInput.value.value = ''
    }
  }
)

const openFilePicker = () => {
  fileInput.value?.click()
}

const handleFileChange = (event: Event) => {
  const target = event.target as HTMLInputElement
  files.value = target.files ? Array.from(target.files) : []
}

const handleClose = () => {
  if (importing.value) return
  emit('close')
}

const readFileAsText = async (sourceFile: File): Promise<string> => {
  if (typeof sourceFile.text === 'function') {
    return sourceFile.text()
  }
  if (typeof sourceFile.arrayBuffer === 'function') {
    const buffer = await sourceFile.arrayBuffer()
    return new TextDecoder().decode(buffer)
  }
  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error || new Error('Failed to read file'))
    reader.readAsText(sourceFile)
  })
}

const handleImport = async () => {
  if (!form.content.trim() && files.value.length === 0) {
    appStore.showError(t('admin.accounts.chatgptSessionImportEmpty'))
    return
  }

  importing.value = true
  try {
    const contents = await Promise.all(files.value.map(readFileAsText))
    const payload = {
      content: form.content.trim() || undefined,
      contents,
      notes: form.notes.trim() || undefined,
      group_ids: form.group_ids,
      proxy_id: form.proxy_id,
      concurrency: form.concurrency,
      priority: form.priority,
      rate_multiplier: form.rate_multiplier,
      load_factor: form.load_factor,
      expires_at: form.expires_at,
      auto_pause_on_expired: form.auto_pause_on_expired
    }

    const res = await adminAPI.accounts.importChatGPTSession(payload)
    result.value = res

    const messageParams = {
      total: res.total,
      created: res.created,
      failed: res.failed
    }

    if (res.failed > 0) {
      appStore.showError(t('admin.accounts.chatgptSessionImportPartial', messageParams))
      return
    }

    appStore.showSuccess(t('admin.accounts.chatgptSessionImportSuccess', messageParams))
    emit('imported')
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.chatgptSessionImportFailed'))
  } finally {
    importing.value = false
  }
}
</script>
