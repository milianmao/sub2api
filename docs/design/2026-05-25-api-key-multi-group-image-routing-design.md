# 单 Key 多分组授权与生图自动选组设计

## 背景

当前 API Key 只能通过 `api_keys.group_id` 绑定一个分组。这样可以满足普通文本请求，但无法覆盖以下场景：一个 Key 默认走 OpenAI 普通分组，同时在生图请求时自动走 OpenAI 生图分组。

本设计目标是支持「一个 API Key 授权多个分组，但每次请求仍只解析出一个生效分组」。生效分组用于订阅校验、余额检查、倍率、RPM、调度和用量日志。

## 目标

- 一个 API Key 可以授权多个分组。
- 保留 `api_keys.group_id`，作为默认生效分组，兼容现有行为。
- 生图请求自动从授权分组中选择 OpenAI 生图分组。
- 普通请求继续使用默认分组，不误走生图分组。
- 订阅、计费、调度、上下文和用量日志都使用本次请求解析出的生效分组。

## 非目标

- 不让单次请求同时命中多个分组。
- 不在第一版支持用户通过 header 或 query 手动指定分组。
- 不移除 `api_keys.group_id`。
- 不把所有分组选择逻辑扩展成通用路由系统。本次只解决 API Key 多分组授权和 OpenAI 生图自动选组。

## 数据模型

保留现有字段：

```text
api_keys.group_id
```

含义调整为：默认生效分组。非生图请求默认走它，兼容当前逻辑。

新增中间表：

```text
api_key_groups
- api_key_id
- group_id
- priority（默认 50，数值越小优先级越高）
- created_at
```

含义：一个 API Key 被授权访问哪些分组。

创建或更新 Key 时：

- 如果只选 1 个分组：`group_id = 该分组`，`api_key_groups` 也写入该分组。
- 如果选多个分组：`group_id = 默认分组`，`api_key_groups` 写入全部授权分组。
- 如果清空分组：`group_id = null`，清空授权关系。

迁移旧数据时：

- 如果 `api_keys.group_id` 不为空，写入 `api_key_groups(api_key_id, group_id)`。
- 如果 `api_keys.group_id` 为空，不写入 `api_key_groups`。

## 生效分组解析

每个请求仍只有 1 个生效分组。

### 非生图请求

直接使用默认分组：

```text
api_keys.group_id
```

这样普通 OpenAI 请求、Claude 请求和其他文本请求继续保持现有行为。

### 生图请求

生图请求不直接使用默认分组，而是在该 Key 授权的分组中自动选择：

```text
api_key_groups
  → 找到该 Key 授权的所有 group
  → 过滤出可用于 OpenAI 生图的分组
  → 选择一个作为本次请求的生效分组
```

候选分组条件：

```text
group.status = active
group.platform = openai
group.allow_image_generation = true
```

如果有多个候选分组，按以下顺序稳定选择：

1. 如果默认分组本身就是可用生图分组，优先使用默认分组。
2. 否则按 `api_key_groups.priority` 从小到大选择。
3. 如果 `priority` 相同，按 `group_id` 从小到大选择。

如果没有可用生图分组，返回：

```text
403 IMAGE_GROUP_NOT_AUTHORIZED
```

不会回退到普通 OpenAI 分组。

## 生图请求识别范围

统一实现一个生图请求识别器，不在各入口分散判断：

```text
IsOpenAIImageRequest(request, route, upstreamHint) bool
```

第一版覆盖以下入口：

```text
OpenAI 生图请求识别范围
├─ Images API
│  ├─ POST /v1/images/generations
│  ├─ POST /v1/images/edits
│  └─ POST /v1/images/variations
│
├─ OpenAI Responses 生图
│  └─ POST /v1/responses
│     └─ 请求体包含 image_generation / image_generation_call 等生图工具或输出类型
│
├─ ChatGPT Web 生图
│  └─ 项目内对应的 ChatGPT Web image 入口
│     └─ 依据现有 handler / upstream 标识判断
│
└─ Codex Responses 生图
   └─ 项目内对应的 Codex Responses image 入口
      └─ 依据现有 handler / upstream 标识判断
```

`POST /v1/responses` 只有在请求体或上游提示明确包含生图意图时才算生图请求，避免普通 Responses 文本请求误走生图分组。

## 中间件与计费流程

生效分组解析放在 API Key 鉴权成功之后、订阅和余额检查之前：

```text
提取 API Key
→ 校验 Key / 用户 / IP
→ 加载 Key 授权分组
→ 解析本次请求的生效分组
→ 校验生效分组状态
→ 按生效分组做订阅 / 余额 / 配额检查
→ 将生效分组写入 request context
→ 进入 handler / gateway
```

关键规则：

- `apiKey.Group` 设置为本次请求的生效分组。
- `apiKey.GroupID` 同步设为生效分组 ID，保证后续旧代码继续按单分组读取。
- 数据库里的默认分组不会因为某次生图请求被自动修改。
- `ctxkey.Group` 写入生效分组。
- 订阅模式下，`GetActiveSubscription(userID, groupID)` 使用生效分组 ID。
- `usage_logs.group_id` 记录生效分组，而不是默认分组。

以下两个认证中间件都需要统一接入：

```text
backend/internal/server/middleware/api_key_auth.go
backend/internal/server/middleware/api_key_auth_google.go
```

Google 风格认证中间件主要服务 Gemini 原生入口，第一版不需要主动触发生图自动选组；但仍应复用同一套生效分组解析入口，避免两套中间件在默认分组、上下文和计费行为上分叉。

## API、DTO 与前端交互

对外接口保持兼容，同时新增多分组字段。

API Key 返回结构保留：

```json
{
  "group_id": 1,
  "group": {}
}
```

含义仍是默认分组。

新增字段：

```json
{
  "group_ids": [1, 2],
  "groups": []
}
```

含义是该 Key 授权访问的全部分组。

创建 API Key 继续支持旧参数：

```json
{
  "name": "my-key",
  "group_id": 1
}
```

新增支持：

```json
{
  "name": "my-key",
  "group_id": 1,
  "group_ids": [1, 2]
}
```

规则：

- `group_id` 是默认分组。
- `group_ids` 是授权分组集合。
- 如果传了 `group_ids`，必须包含 `group_id`。
- 如果只传 `group_id`，后端自动把它写入 `group_ids`。
- 如果 `group_id = null`，则 `group_ids` 必须为空或不传。

前端创建 / 编辑弹窗从单选分组改为：

```text
默认分组：单选
授权分组：多选
```

交互规则：

- 默认分组自动加入授权分组。
- 授权分组取消默认分组时，需要先选择新的默认分组。
- 列表显示默认分组，并展示授权分组数量或名称摘要。

## 缓存与失效策略

API Key 鉴权缓存需要从单分组扩展为：

```text
DefaultGroupID
DefaultGroup
AuthorizedGroups
```

其中：

- `DefaultGroupID` 对应数据库里的 `api_keys.group_id`。
- `DefaultGroup` 是默认分组快照。
- `AuthorizedGroups` 是该 Key 授权的全部分组快照，包括默认分组。

请求进来后：

```text
从缓存还原 API Key
→ 根据请求类型从 AuthorizedGroups 中选择生效分组
→ 写回 apiKey.Group / apiKey.GroupID
```

当前缓存版本是 `apiKeyAuthSnapshotVersion = 12`。实现时需要升到新版本，例如 `13`，避免旧缓存结构被误用。

现有失效方式继续保留：

```text
按 key 失效
按 user 失效
按 group 失效
```

其中「按 group 失效」要覆盖两类 Key：

```text
api_keys.group_id = group_id
或
api_key_groups.group_id = group_id
```

某个分组配置变更时，要清掉默认分组指向它的 Key，以及授权分组包含它的 Key。

用户分组 RPM Override 第一版不全量塞入缓存。生效分组确定后，如果缓存中的 override 对应本次生效分组就直接使用；否则沿用现有回退查询逻辑。后续如有性能压力，再扩展为 `map[group_id]override`。

## 错误处理与边界规则

### Key 没有任何分组

如果 API Key 没有关联默认分组，也没有授权分组：

- 普通请求：按现有逻辑继续允许无分组 Key，后续走系统默认行为。
- 生图请求：返回 `403 IMAGE_GROUP_NOT_AUTHORIZED`。

### 默认分组不可用

如果 `api_keys.group_id` 指向的默认分组被删除或停用：

- 普通请求：返回现有错误，例如 `GROUP_DELETED` / `GROUP_DISABLED`。
- 生图请求：继续在授权分组中查找其他可用生图分组。

### 授权分组不可用

自动选组时过滤掉：

```text
status != active
deleted
不允许生图
platform != openai
```

### 多分组但没有匹配能力

例如 Key 授权了 OpenAI 普通分组、Claude 分组和 Gemini 分组，但请求是 OpenAI 生图，则返回：

```text
403 IMAGE_GROUP_NOT_AUTHORIZED
```

不会回退到普通 OpenAI 分组。

### 默认分组不在授权集合中

创建 / 更新时禁止这种状态：

```text
400 DEFAULT_GROUP_NOT_AUTHORIZED
```

后端强校验，前端只做辅助限制。

## 测试策略

后端测试覆盖：

1. **迁移兼容**
   - 旧 Key 有 `group_id` 时，迁移后 `api_key_groups` 自动包含该分组。
   - 旧 Key 没有 `group_id` 时，不产生授权分组。

2. **创建 / 更新校验**
   - 只传 `group_id` 时，自动补齐 `group_ids`。
   - 传多个 `group_ids` 时，要求包含默认 `group_id`。
   - 默认分组不在授权集合时返回 `DEFAULT_GROUP_NOT_AUTHORIZED`。

3. **生效分组解析**
   - 普通请求使用默认分组。
   - Images API 生图自动选择授权生图分组。
   - OpenAI Responses 生图自动选择授权生图分组。
   - ChatGPT Web 生图自动选择授权生图分组。
   - Codex Responses 生图自动选择授权生图分组。
   - 没有授权生图分组时返回 `IMAGE_GROUP_NOT_AUTHORIZED`。
   - 多个生图分组时按「默认分组 → priority → group_id」稳定选择。

4. **计费与上下文**
   - 订阅检查使用生效分组。
   - `ctxkey.Group` 是生效分组。
   - 用量日志写入生效分组 ID。
   - 普通请求不误用生图分组。

5. **缓存失效**
   - Key 更新授权分组后，鉴权缓存失效。
   - 分组配置变更后，默认绑定和授权绑定的 Key 都失效。
   - 缓存版本升级后旧 snapshot 不再复用。

前端验收覆盖：

- 创建 Key 时可选择多个授权分组。
- 默认分组始终包含在授权分组内。
- 编辑 Key 时能回显默认分组和授权分组。
- 列表能展示默认分组与授权分组摘要。
- 旧接口字段 `group_id` 的展示不受影响。

## 手工验收场景

创建两个 OpenAI 分组：

1. OpenAI 普通分组：`allow_image_generation = false`。
2. OpenAI 生图分组：`allow_image_generation = true`。

创建一个 API Key：

- 默认分组 = OpenAI 普通分组。
- 授权分组 = OpenAI 普通分组 + OpenAI 生图分组。

验证：

- 普通 OpenAI 文本请求走普通分组。
- `/v1/images/generations` 走生图分组。
- `/v1/responses` 生图走生图分组。
- ChatGPT Web 生图走生图分组。
- Codex Responses 生图走生图分组。
- 移除生图分组授权后，生图请求返回 `403 IMAGE_GROUP_NOT_AUTHORIZED`。
