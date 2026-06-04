<template>
  <AppLayout>
    <div class="space-y-6 p-4 md:p-6">
      <section class="overflow-hidden rounded-3xl border border-amber-200/70 bg-gradient-to-br from-amber-50 via-white to-orange-50 shadow-sm dark:border-amber-900/40 dark:from-amber-950/20 dark:via-dark-900 dark:to-orange-950/10">
        <div class="grid gap-6 px-6 py-8 lg:grid-cols-[1.3fr_0.9fr] lg:px-8">
          <div class="space-y-5">
            <div class="flex flex-wrap gap-2">
              <span class="rounded-full bg-gray-900 px-3 py-1 text-xs font-medium text-white dark:bg-white dark:text-gray-900">Admin Tool</span>
              <span class="rounded-full bg-emerald-100 px-3 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">Token Ephemeral</span>
              <span class="rounded-full bg-sky-100 px-3 py-1 text-xs font-medium text-sky-700 dark:bg-sky-900/30 dark:text-sky-300">Proxy Optional</span>
            </div>
            <div>
              <h1 class="text-3xl font-semibold tracking-tight text-gray-900 dark:text-white">ChatGPT Plus 支付长链生成器</h1>
              <p class="mt-2 max-w-2xl text-sm leading-6 text-gray-600 dark:text-gray-300">
                仅管理员可用。accessToken 只用于本次请求，不会保存到浏览器、本地存储或后端数据库。
              </p>
            </div>
          </div>
          <div class="grid gap-3 sm:grid-cols-3 lg:grid-cols-1">
            <div class="rounded-2xl border border-white/80 bg-white/80 p-4 backdrop-blur dark:border-white/10 dark:bg-white/5">
              <div class="text-xs uppercase tracking-[0.18em] text-gray-500 dark:text-gray-400">Checkout Target</div>
              <div class="mt-2 text-sm font-medium text-gray-900 dark:text-white">固定 ChatGPT Plus Hosted Checkout</div>
            </div>
            <div class="rounded-2xl border border-white/80 bg-white/80 p-4 backdrop-blur dark:border-white/10 dark:bg-white/5">
              <div class="text-xs uppercase tracking-[0.18em] text-gray-500 dark:text-gray-400">Proxy Mode</div>
              <div class="mt-2 text-sm font-medium text-gray-900 dark:text-white">直连 / 代理池 / API 提取</div>
            </div>
            <div class="rounded-2xl border border-white/80 bg-white/80 p-4 backdrop-blur dark:border-white/10 dark:bg-white/5">
              <div class="text-xs uppercase tracking-[0.18em] text-gray-500 dark:text-gray-400">Result</div>
              <div class="mt-2 text-sm font-medium text-gray-900 dark:text-white">支持复制、手动打开与自动打开</div>
            </div>
          </div>
        </div>
      </section>

      <div class="grid gap-6 xl:grid-cols-[minmax(0,1.3fr)_minmax(320px,0.9fr)]">
        <section class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="space-y-5">
            <div>
              <div class="text-sm font-medium text-gray-900 dark:text-white">生成表单</div>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">选择出站方式，提交临时 accessToken，生成可打开的 ChatGPT Plus 支付长链。</p>
            </div>

            <div class="space-y-2">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">accessToken</label>
              <div class="relative">
                <input
                  v-model="form.accessToken"
                  :type="showAccessToken ? 'text' : 'password'"
                  class="form-input w-full pr-24"
                  placeholder="粘贴 ChatGPT accessToken"
                />
                <div class="absolute inset-y-0 right-2 flex items-center gap-1">
                  <button type="button" class="btn btn-secondary btn-sm" @click="showAccessToken = !showAccessToken">
                    {{ showAccessToken ? '隐藏' : '显示' }}
                  </button>
                  <button type="button" class="btn btn-secondary btn-sm" :disabled="!form.accessToken" @click="form.accessToken = ''">
                    清空
                  </button>
                </div>
              </div>
              <p class="text-xs text-gray-500 dark:text-gray-400">不会保存 accessToken；刷新页面后会自动清空。</p>
            </div>

            <div class="grid gap-4 lg:grid-cols-2">
              <div class="space-y-2">
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">代理模式</label>
                <select v-model="form.proxySource" class="form-select w-full">
                  <option value="direct">直连</option>
                  <option value="pool">代理池</option>
                  <option value="extract_api">API 提取</option>
                </select>
              </div>

              <div class="space-y-2">
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">自动打开</label>
                <label class="flex h-11 items-center justify-between rounded-2xl border border-gray-200 px-4 dark:border-dark-700">
                  <span class="text-sm text-gray-700 dark:text-gray-300">生成成功后自动打开</span>
                  <input v-model="form.autoOpen" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" />
                </label>
              </div>
            </div>

            <div v-if="form.proxySource === 'pool'" class="space-y-2">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">选择 active 代理</label>
              <ProxySelector v-model="form.proxyId" :proxies="activeProxies" />
              <p v-if="!activeProxies.length" class="text-sm text-amber-600 dark:text-amber-300">当前没有 active 代理，可前往 /admin/proxies 添加，或改用直连 / API 提取。</p>
            </div>

            <div v-if="form.proxySource === 'extract_api'" class="space-y-2">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">提取 API URL</label>
              <input
                v-model="form.extractApiUrl"
                type="url"
                class="form-input w-full"
                placeholder="https://example.com/get-proxy"
              />
              <p class="text-xs text-gray-500 dark:text-gray-400">仅保存此非敏感偏好。返回内容需为 host:port 或 protocol://host:port。</p>
            </div>

            <div class="flex flex-wrap items-center gap-3">
              <button type="button" class="btn btn-primary" :disabled="submitting" @click="handleSubmit">
                {{ submitting ? '生成中...' : '生成支付长链' }}
              </button>
              <span v-if="statusText" class="text-sm text-gray-500 dark:text-gray-400">{{ statusText }}</span>
            </div>
          </div>
        </section>

        <section class="space-y-4">
          <div class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="text-sm font-medium text-gray-900 dark:text-white">当前出站方式</div>
            <div v-if="form.proxySource === 'direct'" class="mt-3 rounded-2xl border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-800 dark:border-emerald-900/40 dark:bg-emerald-900/20 dark:text-emerald-200">
              当前将不使用代理，直接请求固定的 ChatGPT Checkout 接口。
            </div>
            <div v-else-if="form.proxySource === 'pool'" class="mt-3 space-y-3">
              <div v-if="selectedProxy" class="rounded-2xl border border-sky-200 bg-sky-50 p-4 dark:border-sky-900/40 dark:bg-sky-900/20">
                <div class="text-sm font-medium text-sky-900 dark:text-sky-100">{{ selectedProxy.name }}</div>
                <div class="mt-2 text-xs text-sky-700 dark:text-sky-300">{{ selectedProxy.protocol }}://{{ selectedProxy.host }}:{{ selectedProxy.port }}</div>
                <div class="mt-3 grid gap-2 text-xs text-sky-800 dark:text-sky-200 sm:grid-cols-2">
                  <div>账号数：{{ selectedProxy.account_count ?? 0 }}</div>
                  <div>延迟：{{ selectedProxy.latency_ms ? `${selectedProxy.latency_ms} ms` : '未知' }}</div>
                  <div>地区：{{ selectedProxy.country || selectedProxy.region || '未知' }}</div>
                  <div>质量：{{ selectedProxy.quality_grade || selectedProxy.quality_status || '未检测' }}</div>
                </div>
              </div>
              <div v-else class="rounded-2xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-200">
                请选择一个 active 代理。
              </div>
            </div>
            <div v-else class="mt-3 rounded-2xl border border-violet-200 bg-violet-50 p-4 dark:border-violet-900/40 dark:bg-violet-900/20">
              <div class="text-sm font-medium text-violet-900 dark:text-violet-100">API 提取</div>
              <div class="mt-2 text-xs text-violet-700 dark:text-violet-300">Host：{{ extractApiHost || '未填写' }}</div>
              <div class="mt-1 text-xs text-violet-700 dark:text-violet-300">协议：{{ extractApiProtocol || '未填写' }}</div>
              <div class="mt-3 text-xs text-violet-700 dark:text-violet-300">提取结果只会被解析为代理，不会被当作 checkout 目标地址。</div>
            </div>
          </div>

          <div class="rounded-3xl border border-gray-900 bg-gray-950 p-5 text-gray-100 shadow-sm">
            <div class="flex items-center justify-between gap-3">
              <div>
                <div class="text-sm font-medium text-white">结果区</div>
                <p class="mt-1 text-xs text-gray-400">请求中会显示阶段状态；成功后支持复制和打开。</p>
              </div>
              <div v-if="result.generatedAt" class="text-xs text-gray-500">{{ result.generatedAt }}</div>
            </div>

            <div v-if="result.url" class="mt-4 space-y-4">
              <div class="rounded-2xl border border-emerald-500/30 bg-emerald-500/10 p-4 text-sm text-emerald-100">支付长链生成成功。</div>
              <div class="rounded-2xl border border-gray-800 bg-black/40 p-4">
                <div class="break-all font-mono text-sm text-gray-100">{{ result.url }}</div>
              </div>
              <div class="flex flex-wrap gap-2">
                <button type="button" class="btn btn-secondary" @click="handleCopy(result.url)">复制长链</button>
                <button type="button" class="btn btn-primary" @click="openUrl(result.url)">手动打开</button>
              </div>
            </div>

            <div v-else-if="result.error" class="mt-4 rounded-2xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-100">
              {{ result.error }}
            </div>
            <div v-else class="mt-4 rounded-2xl border border-gray-800 bg-black/30 p-4 text-sm text-gray-400">
              {{ statusText || '等待生成支付长链。' }}
            </div>
          </div>
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import { adminAPI } from '@/api/admin'
import type { Proxy } from '@/types'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores'

const PROXY_SOURCE_STORAGE_KEY = 'chatgpt_plus_checkout_proxy_source'
const PROXY_ID_STORAGE_KEY = 'chatgpt_plus_checkout_proxy_id'
const EXTRACT_API_URL_STORAGE_KEY = 'chatgpt_plus_checkout_extract_api_url'
const AUTO_OPEN_STORAGE_KEY = 'chatgpt_plus_checkout_auto_open'

const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const proxies = ref<Proxy[]>([])
const submitting = ref(false)
const loadingProxies = ref(false)
const statusText = ref('')
const showAccessToken = ref(false)
const form = reactive({
  accessToken: '',
  proxySource: 'direct' as 'direct' | 'pool' | 'extract_api',
  proxyId: null as number | null,
  extractApiUrl: '',
  autoOpen: false
})
const result = reactive({
  url: '',
  error: '',
  generatedAt: ''
})

const activeProxies = computed(() => proxies.value.filter(proxy => proxy.status === 'active'))
const selectedProxy = computed(() => activeProxies.value.find(proxy => proxy.id === form.proxyId) ?? null)
const extractApiMeta = computed(() => {
  try {
    if (!form.extractApiUrl) return { host: '', protocol: '' }
    const parsed = new URL(form.extractApiUrl)
    return { host: parsed.host, protocol: parsed.protocol.replace(':', '') }
  } catch {
    return { host: '', protocol: '' }
  }
})
const extractApiHost = computed(() => extractApiMeta.value.host)
const extractApiProtocol = computed(() => extractApiMeta.value.protocol)

const resetResult = () => {
  result.url = ''
  result.error = ''
  result.generatedAt = ''
}

function loadPreferences() {
  const savedSource = localStorage.getItem(PROXY_SOURCE_STORAGE_KEY)
  if (savedSource === 'direct' || savedSource === 'pool' || savedSource === 'extract_api') {
    form.proxySource = savedSource
  }
  const savedProxyId = localStorage.getItem(PROXY_ID_STORAGE_KEY)
  if (savedProxyId) {
    const parsed = Number(savedProxyId)
    form.proxyId = Number.isFinite(parsed) ? parsed : null
  }
  form.extractApiUrl = localStorage.getItem(EXTRACT_API_URL_STORAGE_KEY) ?? ''
  form.autoOpen = localStorage.getItem(AUTO_OPEN_STORAGE_KEY) === 'true'
  form.accessToken = ''
}

function persistPreferences() {
  localStorage.setItem(PROXY_SOURCE_STORAGE_KEY, form.proxySource)
  if (form.proxyId == null) {
    localStorage.removeItem(PROXY_ID_STORAGE_KEY)
  } else {
    localStorage.setItem(PROXY_ID_STORAGE_KEY, String(form.proxyId))
  }
  localStorage.setItem(EXTRACT_API_URL_STORAGE_KEY, form.extractApiUrl)
  localStorage.setItem(AUTO_OPEN_STORAGE_KEY, String(form.autoOpen))
}

async function loadProxies() {
  loadingProxies.value = true
  try {
    proxies.value = await adminAPI.proxies.getAllWithCount()
    if (form.proxySource === 'pool') {
      const matched = activeProxies.value.find(proxy => proxy.id === form.proxyId)
      if (!matched) {
        form.proxyId = activeProxies.value[0]?.id ?? null
      }
    }
  } catch (error) {
    console.error('Failed to load proxies:', error)
    appStore.showError('加载代理列表失败')
  } finally {
    loadingProxies.value = false
  }
}

function validateForm(): string | null {
  if (!form.accessToken.trim()) {
    return '请粘贴 accessToken，系统不会保存它。'
  }
  if (form.proxySource === 'pool' && !form.proxyId) {
    return activeProxies.value.length ? '请选择一个代理。' : '当前没有 active 代理，请改用直连或 API 提取。'
  }
  if (form.proxySource === 'extract_api') {
    if (!form.extractApiUrl.trim()) {
      return '请填写提取 API URL。'
    }
    try {
      const parsed = new URL(form.extractApiUrl)
      if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
        return '提取 API 仅支持 http/https。'
      }
    } catch {
      return '提取 API URL 格式不正确。'
    }
  }
  return null
}

function openUrl(url: string) {
  window.open(url, '_blank', 'noopener,noreferrer')
}

async function handleCopy(url: string) {
  await copyToClipboard(url, '支付长链已复制')
}

async function handleSubmit() {
  resetResult()
  const validationMessage = validateForm()
  if (validationMessage) {
    result.error = validationMessage
    appStore.showError(validationMessage)
    return
  }

  submitting.value = true
  try {
    statusText.value = '准备出站方式...'
    const payload: {
      access_token: string
      proxy_source: 'direct' | 'pool' | 'extract_api'
      proxy_id?: number
      extract_api_url?: string
    } = {
      access_token: form.accessToken.trim(),
      proxy_source: form.proxySource
    }
    if (form.proxySource === 'pool' && form.proxyId) {
      payload.proxy_id = form.proxyId
    }
    if (form.proxySource === 'extract_api') {
      payload.extract_api_url = form.extractApiUrl.trim()
    }

    statusText.value = '请求 checkout...'
    const response = await adminAPI.chatgptPlusCheckout.createCheckoutLink(payload)
    statusText.value = '等待长链...'
    result.url = response.url
    result.generatedAt = new Date().toLocaleString()
    persistPreferences()
    statusText.value = '已生成支付长链。'

    if (form.autoOpen && response.url) {
      setTimeout(() => {
        openUrl(response.url)
      }, 600)
    }
  } catch (error: any) {
    const message = error?.response?.data?.message || error?.message || '生成支付长链失败，请稍后重试。'
    console.error('Failed to create checkout link:', message)
    result.error = message
    statusText.value = ''
  } finally {
    submitting.value = false
  }
}

watch(() => form.proxySource, (source) => {
  resetResult()
  if (source === 'pool' && !selectedProxy.value) {
    form.proxyId = activeProxies.value[0]?.id ?? null
  }
})

onMounted(async () => {
  loadPreferences()
  await loadProxies()
  if (form.proxySource === 'pool' && !form.proxyId) {
    form.proxyId = activeProxies.value[0]?.id ?? null
  }
})
</script>
