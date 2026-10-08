# ChatGPT Plus 支付长链生成器设计

## 背景与目标

新增一个仅管理员可用的单页工具，用于使用 ChatGPT accessToken 生成 ChatGPT Plus Stripe 支付长链。页面提供精致支付工作室风格 UI，支持直连、现有代理池选择、代理提取 API 三种出站方式。

目标是提供一个边界清晰的管理员工具：accessToken 只用于本次请求，不保存到浏览器或后端；checkout 目标固定为 ChatGPT 支付接口；后端不接收任意 checkout URL。

## 范围

### 包含

- 新增管理员页面 `/admin/chatgpt-plus-checkout`。
- 新增管理员接口 `POST /api/v1/admin/chatgpt-plus-checkout`。
- 前端支持三种代理模式：
  - 直连。
  - 从现有 active 代理池选择一个代理。
  - 使用提取 API URL 临时提取代理。
- 前端保存非敏感偏好：上次代理模式、上次代理 ID、提取 API URL、是否自动打开。
- 后端使用固定 ChatGPT Plus checkout payload 生成长链。
- 成功后展示长链、复制按钮、手动打开按钮；可选自动打开。

### 不包含

- 不新增数据库表。
- 不保存 accessToken。
- 不把提取 API URL 或提取出的代理写入全局代理池。
- 不做通用 HTTP 请求工具。
- 不允许前端自定义 checkout endpoint 或 payload。

## 架构

### 前端

新增 `frontend/src/views/admin/ChatGPTPlusCheckoutView.vue`。路由注册到 `frontend/src/router/index.ts`，路径为 `/admin/chatgpt-plus-checkout`，要求管理员身份。

页面复用现有后台布局和基础组件：`AppLayout`、`btn`、`input`、`Select`、`Icon`、`BaseDialog` 或同类组件。页面视觉方向采用浅色暖调、深色结果面板、卡片分区的“精致支付工作室”风格。

页面从现有代理接口拉取代理池：`adminAPI.proxies.getAllWithCount()`。代理池模式只展示 active 代理，并复用返回的账号数量、延迟和质量字段做摘要展示。

### 后端

新增管理员 handler，注册到 `/api/v1/admin/chatgpt-plus-checkout`，复用现有 adminAuth。接口只负责：校验请求、解析出站方式、创建临时 HTTP client、向固定 ChatGPT checkout endpoint 发起请求、返回 URL 或安全错误摘要。

请求体：

```json
{
  "access_token": "chatgpt access token",
  "proxy_source": "direct | pool | extract_api",
  "proxy_id": 123,
  "extract_api_url": "https://example.com/get-proxy"
}
```

字段规则：`proxy_source` 必填；`proxy_id` 仅在 `proxy_source=pool` 时必填；`extract_api_url` 仅在 `proxy_source=extract_api` 时必填。

返回体：

```json
{
  "url": "https://..."
}
```

## UI 设计

### 顶部 Hero

- 标题：`ChatGPT Plus 支付长链生成器`。
- 副标题：说明这是管理员工具，accessToken 不会保存。
- 状态徽章：`Admin Tool`、`Token Ephemeral`、`Proxy Optional`。

### 左侧生成表单

- accessToken 密码输入框：支持显示/隐藏、清空。
- 代理模式选择：`直连`、`代理池`、`API 提取`。
- 代理池模式：显示可搜索代理选择器，代理条目展示名称、协议、地址、地区、延迟、质量状态。
- API 提取模式：显示提取 API URL 输入框和保存提示。
- 自动打开开关：生成成功后倒计时打开新窗口，默认从 localStorage 读取。
- 主按钮：`生成支付长链`。

### 右侧状态卡片

- 直连模式：展示“当前将不使用代理”。
- 代理池模式：展示所选代理摘要。
- API 提取模式：展示提取 API URL 的 host、协议、安全提示。

### 结果区

- 请求中：显示阶段状态：准备出站方式、请求 checkout、等待长链。
- 成功：显示长链、复制按钮、手动打开按钮、生成时间。
- 失败：显示错误摘要和建议操作，不展示 token、代理密码或完整 Authorization header。

## 数据流

### 页面加载

1. 拉取 active 代理列表。
2. 读取 localStorage：
   - `chatgpt_plus_checkout_proxy_source`
   - `chatgpt_plus_checkout_proxy_id`
   - `chatgpt_plus_checkout_extract_api_url`
   - `chatgpt_plus_checkout_auto_open`
3. 如果上次选择的代理仍存在且 active，则自动选中；否则代理池模式选中第一个 active 代理。
4. accessToken 始终为空。

### 生成流程

1. 前端校验 accessToken 非空。
2. 根据代理模式校验：
   - 直连：无需额外字段。
   - 代理池：必须选择代理。
   - API 提取：必须填写 http/https 提取 API URL。
3. 调用管理员生成接口。
4. 成功后保存非敏感偏好，展示长链。
5. 如果开启自动打开，则倒计时后调用 `window.open(url, '_blank', 'noopener,noreferrer')`。

### 后端出站流程

固定 checkout endpoint：`https://chatgpt.com/backend-api/payments/checkout`。

固定 payload：

```json
{
  "plan_name": "chatgptplusplan",
  "billing_details": {
    "country": "ID",
    "currency": "IDR"
  },
  "cancel_url": "https://chatgpt.com/#pricing",
  "promo_campaign": {
    "promo_campaign_id": "plus-1-month-free",
    "is_coupon_from_query_param": false
  },
  "checkout_ui_mode": "hosted"
}
```

出站方式：

- `direct`：使用默认 HTTP client。
- `pool`：通过 `proxy_id` 查询现有代理池，要求代理 active 且协议支持。
- `extract_api`：后端先请求 `extract_api_url`，从返回文本中提取代理地址，再使用该代理请求 checkout。

提取 API 返回格式支持：

- `host:port`
- `protocol://host:port`
- `protocol://user:pass@host:port`

没有协议时默认按 `http://host:port` 处理。

## 错误处理与安全

### 前端错误

- accessToken 为空：提示粘贴 accessToken，并说明不会保存。
- 代理池为空：展示空状态，引导前往 `/admin/proxies` 添加代理，或改用直连/API 提取。
- 代理池模式未选择代理：提示选择代理。
- API 提取模式 URL 为空或协议不支持：提示只支持 http/https 提取 API。

### 后端错误

- 非管理员：沿用现有认证错误。
- accessToken 为空：400。
- proxy_source 非法：400。
- proxy_id 不存在或 inactive：400/404。
- 代理协议不支持：400。
- 提取 API URL 协议不是 http/https：400。
- 提取 API 返回内容无法解析为代理：400。
- 上游 checkout 超时：返回安全错误摘要。
- 上游返回非 JSON 或 JSON 中没有 url：返回安全错误摘要。

### 安全约束

- accessToken 不写数据库、不写 localStorage、不写日志。
- 后端日志不记录 Authorization、代理密码、完整请求体。
- 后端只请求固定 checkout endpoint，不接受前端传入 checkout URL。
- 代理池模式只允许使用现有代理池中的代理。
- API 提取模式只把提取结果解析为代理，不把它作为 checkout 目标。
- API 提取请求设置短超时，checkout 请求设置明确超时。

## 测试设计

### 后端测试

- accessToken 为空返回 400。
- `proxy_source=direct` 时不查询代理池，成功返回上游 url。
- `proxy_source=pool` 时使用 active 代理配置。
- inactive 或不存在代理返回错误。
- `proxy_source=extract_api` 时先读取提取 API，再使用提取代理请求 checkout。
- 提取 API 返回 `host:port` 时默认补 `http://`。
- 提取 API URL 非 http/https 返回 400。
- 上游返回 `{ "url": "..." }` 时返回同一 URL。
- 上游错误不会在响应中泄露 accessToken。

### 前端测试

- 页面加载显示直连、代理池、API 提取三种模式。
- accessToken 为空时点击生成不会发请求。
- 直连模式请求体包含 `proxy_source: 'direct'`。
- 代理池模式请求体包含 `proxy_source: 'pool'` 和 `proxy_id`。
- API 提取模式请求体包含 `proxy_source: 'extract_api'` 和 `extract_api_url`。
- 成功后显示长链、复制按钮、手动打开按钮。
- 刷新页面后 accessToken 不保留。
- 非敏感偏好会保存到 localStorage。

### 手动验证

- 运行前端新增测试。
- 运行前端 typecheck。
- 运行后端新增 handler 测试。
- 启动应用，用管理员账号打开 `/admin/chatgpt-plus-checkout` 验证：
  - 不填 token 不提交。
  - 直连、代理池、API 提取三种模式可切换。
  - 成功和失败状态展示正确。
  - 刷新页面后 accessToken 清空。

## 验收标准

- 管理员可以在新页面生成并复制支付长链。
- 三种出站方式均按设计发出请求。
- accessToken 不被持久化。
- 接口不接受任意 checkout URL。
- 页面视觉风格精致，和现有后台样式兼容。
- 自动化测试覆盖核心行为。
