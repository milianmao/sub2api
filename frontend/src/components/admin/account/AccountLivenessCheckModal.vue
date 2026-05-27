<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.liveness.title')"
    width="extra-wide"
    @close="handleClose"
  >
    <div class="space-y-5">
      <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-blue-100 bg-blue-50 p-4 dark:border-blue-900/40 dark:bg-blue-900/20">
        <div>
          <div class="text-sm font-semibold text-blue-900 dark:text-blue-100">
            {{ scopeLabel }}
          </div>
          <div class="mt-1 text-xs text-blue-700 dark:text-blue-300">
            {{ t('admin.accounts.liveness.executionHint', { concurrency: DEFAULT_CONCURRENCY }) }}
          </div>
        </div>
        <button
          data-test="start-liveness-check"
          class="btn btn-primary"
          :disabled="checking || targetCount === 0"
          @click="startCheck"
        >
          <Icon :name="checking ? 'refresh' : 'play'" size="sm" :class="checking ? 'animate-spin' : ''" />
          {{ checking ? t('admin.accounts.liveness.checking') : result || errorMessage ? t('admin.accounts.liveness.retry') : t('admin.accounts.liveness.start') }}
        </button>
      </div>

      <div v-if="errorMessage" class="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800/50 dark:bg-red-900/20 dark:text-red-300">
        {{ errorMessage }}
      </div>

      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <MetricCard icon="chart" tone="blue" :label="t('admin.accounts.liveness.progress')" :value="progressValue" :hint="progressHint" />
        <MetricCard icon="checkCircle" tone="green" :label="t('admin.accounts.liveness.alive')" :value="String(result?.success ?? 0)" :hint="successRateHint" />
        <MetricCard icon="xCircle" tone="red" :label="t('admin.accounts.liveness.failed')" :value="String(result?.failed ?? 0)" :hint="t('admin.accounts.liveness.failedHint')" />
        <MetricCard icon="bolt" tone="violet" :label="t('admin.accounts.liveness.avgLatency')" :value="formatLatency(result?.average_latency_ms ?? 0)" :hint="t('admin.accounts.liveness.avgLatencyHint')" />
      </div>

      <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <div class="card p-4">
          <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.liveness.byPlatform') }}</h3>
          <div v-if="platformRows.length" class="space-y-2">
            <div v-for="row in platformRows" :key="row.platform" class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 text-sm dark:bg-dark-700">
              <span class="font-medium text-gray-700 dark:text-gray-200">{{ row.platform }}</span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.liveness.platformSummary', row) }}
              </span>
            </div>
          </div>
          <div v-else class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accounts.liveness.noData') }}</div>
        </div>

        <div class="card p-4">
          <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.liveness.failureReasons') }}</h3>
          <div v-if="failureReasonRows.length" class="space-y-2">
            <div v-for="row in failureReasonRows" :key="row.reason" class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 text-sm dark:bg-dark-700">
              <span class="font-medium text-gray-700 dark:text-gray-200">{{ formatFailureReason(row.reason) }}</span>
              <span class="text-xs text-red-500">{{ row.count }}</span>
            </div>
          </div>
          <div v-else class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accounts.liveness.noFailures') }}</div>
        </div>
      </div>

      <div class="card overflow-hidden">
        <div class="border-b border-gray-100 px-4 py-3 text-sm font-semibold text-gray-900 dark:border-dark-600 dark:text-white">
          {{ t('admin.accounts.liveness.details') }}
        </div>
        <div class="max-h-80 overflow-auto">
          <table class="min-w-full divide-y divide-gray-100 text-sm dark:divide-dark-600">
            <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-700 dark:text-gray-400">
              <tr>
                <th class="px-4 py-2 text-left">{{ t('admin.accounts.liveness.account') }}</th>
                <th class="px-4 py-2 text-left">{{ t('admin.accounts.liveness.platform') }}</th>
                <th class="px-4 py-2 text-left">{{ t('admin.accounts.liveness.result') }}</th>
                <th class="px-4 py-2 text-left">{{ t('admin.accounts.liveness.latency') }}</th>
                <th class="px-4 py-2 text-left">{{ t('admin.accounts.liveness.statusUpdate') }}</th>
                <th class="px-4 py-2 text-left">{{ t('admin.accounts.liveness.message') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-600">
              <tr v-for="item in result?.items ?? []" :key="item.account_id">
                <td class="px-4 py-2 font-medium text-gray-900 dark:text-white">{{ item.account_name }}</td>
                <td class="px-4 py-2 text-gray-600 dark:text-gray-300">{{ item.platform }} / {{ item.type }}</td>
                <td class="px-4 py-2">
                  <span :class="resultBadgeClass(item.result)">{{ formatResult(item.result) }}</span>
                </td>
                <td class="px-4 py-2 text-gray-600 dark:text-gray-300">{{ formatLatency(item.latency_ms) }}</td>
                <td class="px-4 py-2 text-gray-600 dark:text-gray-300">{{ item.status_before }} → {{ item.status_after }}</td>
                <td class="max-w-xs truncate px-4 py-2 text-gray-600 dark:text-gray-300" :title="item.message">{{ item.message }}</td>
              </tr>
              <tr v-if="!result?.items?.length">
                <td colspan="6" class="px-4 py-8 text-center text-gray-500 dark:text-gray-400">
                  {{ checking ? t('admin.accounts.liveness.waiting') : t('admin.accounts.liveness.notStarted') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" @click="handleClose">{{ t('common.close') }}</button>
        <button
          v-if="result"
          data-test="finish-liveness-check"
          class="btn btn-primary"
          @click="emit('completed')"
        >
          {{ t('admin.accounts.liveness.finishAndRefresh') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type {
  AccountLivenessCheckFilters,
  AccountLivenessCheckResponse,
  AccountLivenessCheckResult
} from '@/api/admin/accounts'

const DEFAULT_CONCURRENCY = 5

const props = defineProps<{
  show: boolean
  selectedIds: number[]
  filters: AccountLivenessCheckFilters
  filteredCount: number
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'completed'): void
}>()

const { t } = useI18n()
const checking = ref(false)
const result = ref<AccountLivenessCheckResponse | null>(null)
const errorMessage = ref('')

const targetCount = computed(() => props.selectedIds.length > 0 ? props.selectedIds.length : props.filteredCount)
const scopeLabel = computed(() => props.selectedIds.length > 0
  ? t('admin.accounts.liveness.scopeSelected', { count: props.selectedIds.length })
  : t('admin.accounts.liveness.scopeFiltered', { count: props.filteredCount }))
const progressValue = computed(() => `${result.value?.completed ?? 0}/${result.value?.total ?? targetCount.value}`)
const progressHint = computed(() => checking.value ? t('admin.accounts.liveness.checking') : t('admin.accounts.liveness.progressHint'))
const successRateHint = computed(() => {
  if (!result.value || result.value.total === 0) return t('admin.accounts.liveness.successRate', { rate: '0.0' })
  return t('admin.accounts.liveness.successRate', { rate: ((result.value.success / result.value.total) * 100).toFixed(1) })
})
const platformRows = computed(() => Object.entries(result.value?.by_platform ?? {}).map(([platform, stats]) => ({ platform, ...stats })))
const failureReasonRows = computed(() => Object.entries(result.value?.failure_reasons ?? {}).map(([reason, count]) => ({ reason, count })))

async function startCheck() {
  checking.value = true
  errorMessage.value = ''
  try {
    result.value = await adminAPI.accounts.livenessCheck(
      props.selectedIds.length > 0
        ? { scope: 'selected', account_ids: props.selectedIds, concurrency: DEFAULT_CONCURRENCY }
        : { scope: 'filtered', filters: props.filters, concurrency: DEFAULT_CONCURRENCY }
    )
  } catch (error: unknown) {
    errorMessage.value = error instanceof Error ? error.message : String(error)
  } finally {
    checking.value = false
  }
}

function handleClose() {
  emit('close')
}

function formatLatency(value: number) {
  return value > 0 ? `${value}ms` : '-'
}

function formatFailureReason(reason: string) {
  return t(`admin.accounts.liveness.failureReason.${reason}`)
}

function formatResult(value: AccountLivenessCheckResult) {
  return t(`admin.accounts.liveness.resultValue.${value}`)
}

function resultBadgeClass(value: AccountLivenessCheckResult) {
  const base = 'inline-flex rounded-full px-2 py-0.5 text-xs font-semibold'
  if (value === 'success') return `${base} bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300`
  if (value === 'failed') return `${base} bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300`
  return `${base} bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300`
}

const toneClasses: Record<string, { wrap: string; icon: string }> = {
  blue: { wrap: 'bg-blue-100 dark:bg-blue-900/30', icon: 'text-blue-600 dark:text-blue-400' },
  green: { wrap: 'bg-green-100 dark:bg-green-900/30', icon: 'text-green-600 dark:text-green-400' },
  red: { wrap: 'bg-red-100 dark:bg-red-900/30', icon: 'text-red-600 dark:text-red-400' },
  violet: { wrap: 'bg-violet-100 dark:bg-violet-900/30', icon: 'text-violet-600 dark:text-violet-400' }
}

const MetricCard = defineComponent({
  props: {
    icon: { type: String, required: true },
    tone: { type: String, required: true },
    label: { type: String, required: true },
    value: { type: String, required: true },
    hint: { type: String, required: true }
  },
  setup(cardProps) {
    return () => h('div', { class: 'card p-4' }, [
      h('div', { class: 'flex items-center gap-3' }, [
        h('div', { class: ['rounded-lg p-2', toneClasses[cardProps.tone]?.wrap] }, [
          h(Icon, { name: cardProps.icon, size: 'md', class: toneClasses[cardProps.tone]?.icon })
        ]),
        h('div', [
          h('p', { class: 'text-xs font-medium text-gray-500 dark:text-gray-400' }, cardProps.label),
          h('p', { class: 'text-xl font-bold text-gray-900 dark:text-white' }, cardProps.value),
          h('p', { class: 'text-xs text-gray-500 dark:text-gray-400' }, cardProps.hint)
        ])
      ])
    ])
  }
})
</script>
