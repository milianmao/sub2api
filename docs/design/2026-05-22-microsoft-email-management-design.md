# 微软邮箱批量管理单页设计

日期：2026-05-22

## 背景

项目需要新增一个管理员单页，用于批量管理 Microsoft 邮箱账号。用户明确要求只做单页管理，不迁移完整邮件客户端；可参考 `Maishan-Inc/Microsoft-Email-Manager` 的能力，但范围限定为导入、健康检查、单账号验证码获取和结果查看。

## 目标

新增 `/admin/microsoft-emails` 管理页，形成以下闭环：

1. 导入 Microsoft 邮箱账号。
2. 列表管理邮箱账号。
3. 检查账号的 Microsoft Graph OAuth 可用性。
4. 对单个账号实时获取邮箱验证码。
5. 展示本次操作结果和失败原因。

## 非目标

- 不做多页邮件客户端。
- 不做完整邮件正文归档。
- 不做 IMAP。
- 不做发信、转发、删除邮件。
- 不做批量获取验证码。
- 不做附件、文件夹管理、高级邮件客户端功能。

## 功能范围

### TXT 导入

支持上传 `.txt` 文件或粘贴文本。每行一个账号，格式固定为：

```text
email----password----client_id----refresh_token
```

示例：

```text
user@example.com----password----client-id----refresh-token
```

导入规则：

- 忽略空行。
- 每行必须拆分出 4 个字段。
- 校验 email 格式。
- `client_id` 和 `refresh_token` 不能为空。
- 按 email 去重；已存在账号更新凭据，新邮箱创建账号。
- 逐行返回成功、更新、失败明细。

`password` 按导入值保存，但验证码获取不依赖它。前端展示时默认脱敏。

### 邮箱列表

列表字段：

- 邮箱
- 状态：`active / invalid / error`
- client_id（脱敏展示）
- 最近健康检查时间
- 最近获取验证码时间
- 最近错误
- 创建时间
- 操作按钮

列表能力：

- 搜索邮箱。
- 按状态筛选。
- 分页。
- 勾选多行。
- 批量健康检查。
- 批量删除。
- 单行健康检查。
- 单行获取验证码。
- 单行删除。

### 健康检查

健康检查使用账号的 `refresh_token + client_id` 刷新 Microsoft access token。

结果处理：

- 刷新成功：账号状态设为 `active`，更新 `last_check_at`，清空 `last_error`。
- token 失效或刷新失败：账号状态设为 `invalid`，写入 `last_error`。
- 网络或 Graph 服务异常：账号状态设为 `error`，写入 `last_error`。

### 单账号验证码获取

点击某一行“获取验证码”后，后端实时：

1. 使用 `refresh_token + client_id` 刷新 access token。
2. 调用 Microsoft Graph 读取收件箱最新邮件。
3. 从邮件主题和正文文本中提取验证码。
4. 返回提取结果。

限制：

- 只支持单账号获取验证码。
- 不提供批量获取验证码入口。
- 不长期保存邮件正文。
- 仅更新 `last_fetch_at` 和必要的状态/错误信息。

返回字段：

- `email`
- `code`
- `source`：`subject` 或 `body`
- `subject`
- `from`
- `received_at`
- `snippet`
- `error`

验证码提取规则：

- 默认支持 4-8 位数字或字母数字验证码。
- 优先使用包含验证码关键词的邮件。
- 优先使用最新邮件中的高置信匹配。
- 获取不到验证码返回 `code_not_found`，不标记账号失效。

## 数据模型

新增独立表：`microsoft_email_accounts`。

字段：

- `id`
- `email`：唯一索引
- `password`
- `client_id`
- `refresh_token`
- `status`
- `last_check_at`
- `last_fetch_at`
- `last_error`
- `notes`
- `created_at`
- `updated_at`

安全要求：

- `password` 和 `refresh_token` 属于敏感字段。
- DTO/API 返回时必须脱敏。
- 获取邮件时不保存邮件正文。

## API 设计

所有接口均为管理员接口，挂在 `/api/v1/admin/microsoft-emails` 下。

### 列表

`GET /api/v1/admin/microsoft-emails`

查询参数：

- `page`
- `page_size`
- `search`
- `status`

返回分页账号列表。

### 导入

`POST /api/v1/admin/microsoft-emails/import`

请求：

```json
{
  "content": "email----password----client_id----refresh_token"
}
```

返回：

```json
{
  "total": 1,
  "created": 1,
  "updated": 0,
  "failed": 0,
  "items": []
}
```

### 健康检查

`POST /api/v1/admin/microsoft-emails/:id/check`

检查单个账号。

`POST /api/v1/admin/microsoft-emails/batch-check`

请求：

```json
{
  "ids": [1, 2, 3]
}
```

批量检查账号。

### 获取验证码

`POST /api/v1/admin/microsoft-emails/:id/fetch-code`

只对单个账号获取验证码。

### 删除

`DELETE /api/v1/admin/microsoft-emails/:id`

删除单个账号。

`POST /api/v1/admin/microsoft-emails/batch-delete`

请求：

```json
{
  "ids": [1, 2, 3]
}
```

批量删除账号。

## 前端设计

新增页面：`frontend/src/views/admin/MicrosoftEmailsView.vue`。

入口改动：

- `frontend/src/router/index.ts` 增加 `/admin/microsoft-emails` 管理员路由。
- `frontend/src/components/layout/AppSidebar.vue` 增加管理员菜单项“微软邮箱”。
- `frontend/src/api/admin/microsoftEmails.ts` 封装 API。
- `frontend/src/api/admin/index.ts` 挂载 `adminAPI.microsoftEmails`。

页面结构：

1. 顶部统计卡片
   - 总邮箱数
   - 正常账号数
   - 异常账号数
   - 最近检查成功数

2. 工具栏
   - 搜索框
   - 状态筛选
   - 导入 TXT
   - 批量健康检查
   - 批量删除

3. 表格
   - 复选框
   - 邮箱
   - 状态
   - client_id 脱敏
   - 最近检查时间
   - 最近获取时间
   - 最近错误
   - 操作：检查、获取验证码、删除

4. 导入弹窗
   - 粘贴 TXT 内容
   - 选择 `.txt` 文件读取
   - 格式说明
   - 逐行导入结果

5. 单账号验证码结果弹窗
   - 邮箱
   - 验证码
   - 邮件标题
   - 发件人
   - 时间
   - 匹配片段
   - 错误信息
   - 复制验证码

复用现有组件：`AppLayout`、`TablePageLayout`、`DataTable`、`Pagination`、`BaseDialog`、`StatusBadge`、`Input`。

## 后端实现设计

建议新增独立模块，避免混入现有 OpenAI/Claude 账号管理逻辑。

组成：

- ent schema：`MicrosoftEmailAccount`
- repository：Microsoft 邮箱账号 CRUD、分页、状态更新
- service：导入解析、Graph token 刷新、健康检查、验证码获取、验证码提取
- admin handler：HTTP 请求绑定与响应
- routes：注册 `/admin/microsoft-emails` 路由
- dto：敏感字段脱敏输出

Microsoft Graph 访问：

- 使用 refresh token 换取 access token。
- 使用 access token 调用 Graph messages 接口读取最新邮件。
- 解析主题、发件人、接收时间、正文预览/正文文本。

## 错误处理

- 导入错误逐行返回，不中断其他行。
- `refresh_token` 失效标记为 `invalid`。
- Graph 请求异常标记为 `error`。
- 未找到验证码返回 `code_not_found`，不改变账号可用状态。
- 批量健康检查返回每个账号的结果。
- API 返回错误信息避免泄露完整 token/password。

## 测试计划

后端：

- TXT 导入解析测试。
- 重复邮箱导入创建/更新测试。
- 敏感字段脱敏测试。
- token 刷新失败处理测试。
- Graph 响应解析测试。
- 验证码提取测试。
- admin 路由注册测试。

前端：

- API 封装路径测试。
- 导入弹窗读取 `.txt` 与粘贴内容测试。
- 单账号获取验证码按钮 loading/结果展示测试。
- 批量健康检查与批量删除按钮状态测试。

验证命令：

- 后端：`go test ./internal/handler/admin ./internal/server/routes ./internal/service ./internal/repository`
- 前端：`pnpm test:run`
- 前端类型检查：`pnpm typecheck`

## 改造后的实现提示词

```text
你是资深全栈工程师，在 D:\code\sub2api 项目中实现“微软邮箱批量管理”管理员单页。

上下文：
- 前端是 Vue 3 + TypeScript + Vite + TailwindCSS，复用现有 AppLayout、TablePageLayout、DataTable、Pagination、BaseDialog、StatusBadge、Input 等组件。
- 后端是 Go + Gin + ent，管理员账号路由集中在 backend/internal/server/routes/admin.go。
- 现有账号管理页可作为表格、导入弹窗、批量操作、API 封装风格参考，但新功能要独立，不混入 OpenAI/Claude 账号管理逻辑。

目标：
新增管理员单页 /admin/microsoft-emails，用于 Microsoft 邮箱批量管理：TXT 导入、列表管理、健康检查、单账号获取邮箱验证码、结果展示。

功能要求：
1. TXT 导入
   - 支持粘贴文本和上传 .txt 文件。
   - 每行格式：email----password----client_id----refresh_token。
   - 忽略空行，逐行校验。
   - email 唯一，已存在则更新 password/client_id/refresh_token，新邮箱则创建。
   - 返回 total/created/updated/failed/items/errors。

2. 邮箱列表
   - 展示邮箱、状态、client_id 脱敏、最近检查时间、最近获取时间、最近错误、创建时间。
   - 支持分页、搜索、状态筛选。
   - 支持勾选、多选、批量健康检查、批量删除。
   - 不支持批量获取验证码。

3. 健康检查
   - 使用 refresh_token + client_id 刷新 Microsoft access token。
   - 成功标记 active；token 失效标记 invalid；网络/Graph 异常标记 error。
   - 写入 last_check_at 和 last_error。

4. 单账号获取验证码
   - 只在单行操作中提供“获取验证码”。
   - 后端实时刷新 token，调用 Microsoft Graph 读取收件箱最新邮件。
   - 从主题和正文中提取 4-8 位数字或字母数字验证码。
   - 返回 email/code/source/subject/from/received_at/snippet/error。
   - 不长期保存邮件正文，不做批量获取验证码。
   - 未找到验证码返回 code_not_found，但不标记账号失效。

5. 安全与脱敏
   - password 和 refresh_token 是敏感字段，API 列表/详情返回必须脱敏。
   - 错误信息不能泄露完整 token/password。
   - 不保存邮件正文。

后端建议文件：
- backend/ent/schema/microsoftemailaccount.go
- backend/internal/repository/microsoft_email_account_repo.go
- backend/internal/service/microsoft_email_service.go
- backend/internal/handler/admin/microsoft_email_handler.go
- backend/internal/server/routes/admin.go
- 必要的 DTO、迁移和测试文件。

前端建议文件：
- frontend/src/api/admin/microsoftEmails.ts
- frontend/src/views/admin/MicrosoftEmailsView.vue
- frontend/src/router/index.ts
- frontend/src/components/layout/AppSidebar.vue
- 必要的类型和测试文件。

验收标准：
- 管理员能进入 /admin/microsoft-emails。
- 能导入 TXT 格式邮箱账号并看到逐行结果。
- 列表能搜索、筛选、分页。
- 能批量健康检查和批量删除。
- 单个账号能获取验证码并弹窗展示结果。
- 不存在批量获取验证码入口。
- API 不返回明文 refresh_token/password。
- 后端相关单测通过，前端 typecheck 和相关测试通过。

约束：
- 不迁移完整 Microsoft-Email-Manager 项目。
- 不实现 IMAP。
- 不实现邮件归档、发信、转发、删除邮件。
- 不新增无关重构。
- 遵循现有代码风格和路由/API 模式。
```
