# 账号存活检测弹窗实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在账号管理页新增“账号存活检测”弹窗，批量真实检测选中或当前筛选账号，展示仪表盘式统计，并在检测完成后回写账号状态。

**架构：** 后端新增一个同步批量检测接口 `POST /api/v1/admin/accounts/liveness-check`，复用 `AccountTestService.RunTestBackground` 做真实轻量请求，并通过 `AdminService.ClearAccountError` / `SetAccountError` 回写状态。前端新增账号存活检测 API、弹窗组件和账号页入口，弹窗负责范围说明、检测执行、统计卡片、平台/失败原因分布和明细展示，完成后通知账号列表刷新。

**技术栈：** Go + Gin + existing service/repository layer；Vue 3 `<script setup>` + TypeScript + Vitest + Vue Test Utils + Tailwind utility classes + existing `BaseDialog`/`Icon`/`adminAPI` patterns。

---

## 文件结构

### 后端

- 修改：`backend/internal/service/admin_service.go`
  - 继续使用 `AdminService.GetAccountsByIDs`、`ListAccounts`、`ClearAccountError`、`SetAccountError`；如测试 stub 编译需要，在接口注释附近保持方法签名不变。
- 修改：`backend/internal/handler/admin/account_handler.go`
  - 新增批量存活检测请求/响应 DTO。
  - 新增 `LivenessCheck` handler。
  - 复用 `listAccountsFiltered` 和 `normalizeInt64IDList`。
- 修改：`backend/internal/server/routes/admin.go`
  - 注册 `POST /admin/accounts/liveness-check`，必须放在 `/:id` 动态路由之前。
- 创建：`backend/internal/handler/admin/account_liveness_check_test.go`
  - 覆盖 selected、filtered、状态回写、汇总统计、失败隔离。

### 前端

- 修改：`frontend/src/api/admin/accounts.ts`
  - 新增 `AccountLivenessCheckRequest` / `AccountLivenessCheckResponse` 类型和 `livenessCheck` 方法。
- 修改：`frontend/src/api/__tests__/admin.accounts.spec.ts`
  - 增加 API 请求路径和请求体测试。
- 创建：`frontend/src/components/admin/account/AccountLivenessCheckModal.vue`
  - 新增弹窗 UI 与检测执行逻辑。
- 创建：`frontend/src/components/admin/account/__tests__/AccountLivenessCheckModal.spec.ts`
  - 覆盖初始、检测中、完成、失败状态。
- 修改：`frontend/src/views/admin/AccountsView.vue`
  - 在更多工具里新增“账号存活检测”入口。
  - 挂载弹窗，传入 `selIds`、当前筛选条件、分页总数。
  - 完成后关闭弹窗并 `reload()`。
- 修改：`frontend/src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts`
  - 增加入口打开弹窗、完成事件触发刷新测试。
- 修改：`frontend/src/i18n/locales/zh.ts`
  - 添加中文文案。
- 修改：`frontend/src/i18n/locales/en.ts`
  - 添加英文文案。

---

## 任务 1：后端批量检测 handler 测试

**文件：**
- 创建：`backend/internal/handler/admin/account_liveness_check_test.go`
- 参考：`backend/internal/handler/admin/account_handler_available_models_test.go`
- 参考：`backend/internal/service/account_test_service.go:1678` 的 `RunTestBackground`

- [ ] **步骤 1：编写失败测试文件**

创建 `backend/internal/handler/admin/account_liveness_check_test.go`，先写测试用 stub 和 selected 场景：

```go
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type livenessAdminService struct {
	*stubAdminService
	accountsByID map[int64]*service.Account
	listed       []service.Account
	clearIDs     []int64
	setErrors    map[int64]string
}

func newLivenessAdminService() *livenessAdminService {
	return &livenessAdminService{
		stubAdminService: newStubAdminService(),
		accountsByID:     map[int64]*service.Account{},
		setErrors:        map[int64]string{},
	}
}

func (s *livenessAdminService) GetAccountsByIDs(_ context.Context, ids []int64) ([]*service.Account, error) {
	result := make([]*service.Account, 0, len(ids))
	for _, id := range ids {
		if acc := s.accountsByID[id]; acc != nil {
			copy := *acc
			result = append(result, &copy)
		}
	}
	return result, nil
}

func (s *livenessAdminService) ListAccounts(_ context.Context, page, pageSize int, platform, accountType, status, search string, groupID int64, privacyMode string, sortBy, sortOrder string) ([]service.Account, int64, error) {
	return s.listed, int64(len(s.listed)), nil
}

func (s *livenessAdminService) ClearAccountError(_ context.Context, id int64) (*service.Account, error) {
	s.clearIDs = append(s.clearIDs, id)
	acc := s.accountsByID[id]
	if acc == nil {
		return nil, errors.New("account not found")
	}
	copy := *acc
	copy.Status = service.StatusActive
	copy.ErrorMessage = ""
	s.accountsByID[id] = &copy
	return &copy, nil
}

func (s *livenessAdminService) SetAccountError(_ context.Context, id int64, errorMsg string) error {
	s.setErrors[id] = errorMsg
	acc := s.accountsByID[id]
	if acc == nil {
		return errors.New("account not found")
	}
	copy := *acc
	copy.Status = service.StatusError
	copy.ErrorMessage = errorMsg
	copy.Schedulable = false
	s.accountsByID[id] = &copy
	return nil
}

type livenessTestRunner struct {
	results map[int64]*service.ScheduledTestResult
}

func (r livenessTestRunner) RunTestBackground(_ context.Context, accountID int64, _ string) (*service.ScheduledTestResult, error) {
	if result := r.results[accountID]; result != nil {
		return result, nil
	}
	return &service.ScheduledTestResult{
		Status:       "failed",
		ErrorMessage: "missing test result",
		LatencyMs:    0,
		StartedAt:    time.Now(),
		FinishedAt:   time.Now(),
	}, nil
}

func setupLivenessRouter(adminSvc service.AdminService, runner accountLivenessTestRunner) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.accountLivenessTestRunner = runner
	router.POST("/api/v1/admin/accounts/liveness-check", handler.LivenessCheck)
	return router
}

func TestAccountHandlerLivenessCheckSelectedUpdatesStatusesAndAggregates(t *testing.T) {
	adminSvc := newLivenessAdminService()
	adminSvc.accountsByID[1] = &service.Account{ID: 1, Name: "claude-ok", Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth, Status: service.StatusError, Schedulable: true}
	adminSvc.accountsByID[2] = &service.Account{ID: 2, Name: "openai-bad", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true}

	router := setupLivenessRouter(adminSvc, livenessTestRunner{results: map[int64]*service.ScheduledTestResult{
		1: {Status: "success", LatencyMs: 120, StartedAt: time.Now(), FinishedAt: time.Now()},
		2: {Status: "failed", ErrorMessage: "401 unauthorized token abc123", LatencyMs: 55, StartedAt: time.Now(), FinishedAt: time.Now()},
	}})

	body := bytes.NewBufferString(`{"scope":"selected","account_ids":[1,2,2],"concurrency":3}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/liveness-check", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data AccountLivenessCheckResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 2, resp.Data.Total)
	require.Equal(t, 2, resp.Data.Completed)
	require.Equal(t, 1, resp.Data.Success)
	require.Equal(t, 1, resp.Data.Failed)
	require.Equal(t, 0, resp.Data.Skipped)
	require.Equal(t, int64(120), resp.Data.AverageLatencyMs)
	require.Equal(t, 1, resp.Data.ByPlatform[service.PlatformAnthropic].Success)
	require.Equal(t, 1, resp.Data.ByPlatform[service.PlatformOpenAI].Failed)
	require.Equal(t, 1, resp.Data.FailureReasons["auth"])
	require.Equal(t, []int64{1}, adminSvc.clearIDs)
	require.Contains(t, adminSvc.setErrors[2], "401 unauthorized")
	require.NotContains(t, adminSvc.setErrors[2], "abc123")
	require.Len(t, resp.Data.Items, 2)
	require.Equal(t, "active", resp.Data.Items[0].StatusAfter)
	require.Equal(t, "error", resp.Data.Items[1].StatusAfter)
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：

```bash
D:/tool/go/bin/go test ./internal/handler/admin -run TestAccountHandlerLivenessCheckSelectedUpdatesStatusesAndAggregates -count=1
```

预期：FAIL，报错包含 `undefined: accountLivenessTestRunner`、`handler.accountLivenessTestRunner undefined`、`handler.LivenessCheck undefined` 或 `undefined: AccountLivenessCheckResponse`。

- [ ] **步骤 3：补充 filtered 和空范围失败测试**

在同一文件追加：

```go
func TestAccountHandlerLivenessCheckFilteredUsesAccountFilters(t *testing.T) {
	adminSvc := newLivenessAdminService()
	adminSvc.listed = []service.Account{
		{ID: 3, Name: "gemini-ok", Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true},
	}
	adminSvc.accountsByID[3] = &adminSvc.listed[0]

	router := setupLivenessRouter(adminSvc, livenessTestRunner{results: map[int64]*service.ScheduledTestResult{
		3: {Status: "success", LatencyMs: 90, StartedAt: time.Now(), FinishedAt: time.Now()},
	}})

	body := bytes.NewBufferString(`{"scope":"filtered","filters":{"platform":"gemini","status":"active","group":"0"}}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/liveness-check", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data AccountLivenessCheckResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Data.Total)
	require.Equal(t, 1, resp.Data.Success)
	require.Equal(t, "gemini-ok", resp.Data.Items[0].AccountName)
}

func TestAccountHandlerLivenessCheckRejectsEmptySelectedScope(t *testing.T) {
	router := setupLivenessRouter(newLivenessAdminService(), livenessTestRunner{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/liveness-check", bytes.NewBufferString(`{"scope":"selected","account_ids":[]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "account_ids is required")
}
```

- [ ] **步骤 4：运行 handler 测试验证仍失败**

运行：

```bash
D:/tool/go/bin/go test ./internal/handler/admin -run 'TestAccountHandlerLivenessCheck' -count=1
```

预期：FAIL，仍因为 handler 和 DTO 未实现。

---

## 任务 2：后端 DTO、批量检测逻辑与路由

**文件：**
- 修改：`backend/internal/handler/admin/account_handler.go`
- 修改：`backend/internal/server/routes/admin.go:302-344`

- [ ] **步骤 1：在 `AccountHandler` 增加测试运行器接口字段**

修改 `backend/internal/handler/admin/account_handler.go` 中 `AccountHandler` 结构体，新增字段：

```go
type accountLivenessTestRunner interface {
	RunTestBackground(ctx context.Context, accountID int64, modelID string) (*service.ScheduledTestResult, error)
}
```

并在 `AccountHandler` struct 内新增：

```go
accountLivenessTestRunner accountLivenessTestRunner
```

在 `NewAccountHandler` 的返回值中设置：

```go
accountLivenessTestRunner: accountTestService,
```

- [ ] **步骤 2：新增请求/响应 DTO 和聚合结构**

在 `account_handler.go` 的批量请求类型附近新增：

```go
type AccountLivenessCheckRequest struct {
	Scope       string                         `json:"scope" binding:"required,oneof=selected filtered"`
	AccountIDs  []int64                        `json:"account_ids"`
	Filters     AccountLivenessCheckFilters    `json:"filters"`
	Concurrency int                            `json:"concurrency"`
}

type AccountLivenessCheckFilters struct {
	Platform    string `json:"platform"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Group       string `json:"group"`
	Search      string `json:"search"`
	PrivacyMode string `json:"privacy_mode"`
	SortBy      string `json:"sort_by"`
	SortOrder   string `json:"sort_order"`
}

type AccountLivenessCheckResponse struct {
	Total            int                                      `json:"total"`
	Completed        int                                      `json:"completed"`
	Success          int                                      `json:"success"`
	Failed           int                                      `json:"failed"`
	Skipped          int                                      `json:"skipped"`
	AverageLatencyMs int64                                    `json:"average_latency_ms"`
	ByPlatform       map[string]AccountLivenessPlatformStats  `json:"by_platform"`
	FailureReasons   map[string]int                           `json:"failure_reasons"`
	Items            []AccountLivenessCheckItem               `json:"items"`
}

type AccountLivenessPlatformStats struct {
	Success int `json:"success"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type AccountLivenessCheckItem struct {
	AccountID    int64  `json:"account_id"`
	AccountName  string `json:"account_name"`
	Platform     string `json:"platform"`
	Type         string `json:"type"`
	Result       string `json:"result"`
	LatencyMs    int64  `json:"latency_ms"`
	StatusBefore string `json:"status_before"`
	StatusAfter  string `json:"status_after"`
	Message      string `json:"message"`
}
```

- [ ] **步骤 3：新增辅助函数**

在 `account_handler.go` 末尾附近新增：

```go
const (
	accountLivenessDefaultConcurrency = 5
	accountLivenessMaxConcurrency     = 10
	accountLivenessMaxAccounts        = 200
)

func normalizeAccountLivenessConcurrency(value int) int {
	if value <= 0 {
		return accountLivenessDefaultConcurrency
	}
	if value > accountLivenessMaxConcurrency {
		return accountLivenessMaxConcurrency
	}
	return value
}

func classifyAccountLivenessFailure(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "401"), strings.Contains(lower, "403"), strings.Contains(lower, "unauthorized"), strings.Contains(lower, "forbidden"), strings.Contains(lower, "token"), strings.Contains(lower, "api key"):
		return "auth"
	case strings.Contains(lower, "429"), strings.Contains(lower, "rate limit"), strings.Contains(lower, "quota"), strings.Contains(lower, "credit"):
		return "rate_limit"
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline exceeded"), strings.Contains(lower, "context deadline"):
		return "timeout"
	default:
		return "other"
	}
}

func sanitizeAccountLivenessMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "检测失败"
	}
	if len(message) > 240 {
		message = message[:240]
	}
	return message
}

func updateAccountLivenessPlatformStats(stats map[string]AccountLivenessPlatformStats, platform string, result string) {
	item := stats[platform]
	switch result {
	case "success":
		item.Success++
	case "failed":
		item.Failed++
	case "skipped":
		item.Skipped++
	}
	stats[platform] = item
}
```

确保 `account_handler.go` imports 包含 `strings`；如果已有 `strings`，不要重复添加。

- [ ] **步骤 4：实现账号范围解析**

在 `account_handler.go` 新增方法：

```go
func (h *AccountHandler) resolveLivenessAccounts(ctx context.Context, req AccountLivenessCheckRequest) ([]*service.Account, error) {
	switch req.Scope {
	case "selected":
		ids := normalizeInt64IDList(req.AccountIDs)
		if len(ids) == 0 {
			return nil, errors.New("account_ids is required")
		}
		if len(ids) > accountLivenessMaxAccounts {
			return nil, fmt.Errorf("too many accounts: max %d", accountLivenessMaxAccounts)
		}
		return h.adminService.GetAccountsByIDs(ctx, ids)
	case "filtered":
		groupID := int64(0)
		if strings.TrimSpace(req.Filters.Group) != "" {
			parsed, err := strconv.ParseInt(req.Filters.Group, 10, 64)
			if err != nil {
				return nil, errors.New("invalid group filter")
			}
			groupID = parsed
		}
		sortBy := req.Filters.SortBy
		if sortBy == "" {
			sortBy = "name"
		}
		sortOrder := req.Filters.SortOrder
		if sortOrder == "" {
			sortOrder = "asc"
		}
		items, err := h.listAccountsFiltered(ctx, req.Filters.Platform, req.Filters.Type, req.Filters.Status, req.Filters.Search, groupID, req.Filters.PrivacyMode, sortBy, sortOrder)
		if err != nil {
			return nil, err
		}
		if len(items) > accountLivenessMaxAccounts {
			return nil, fmt.Errorf("too many accounts: max %d", accountLivenessMaxAccounts)
		}
		accounts := make([]*service.Account, 0, len(items))
		for i := range items {
			account := items[i]
			accounts = append(accounts, &account)
		}
		return accounts, nil
	default:
		return nil, errors.New("invalid scope")
	}
}
```

确保 imports 包含 `errors`、`fmt`、`strconv`；这些通常已存在，缺哪个补哪个。

- [ ] **步骤 5：实现 `LivenessCheck` handler**

在 `account_handler.go` 中新增方法：

```go
func (h *AccountHandler) LivenessCheck(c *gin.Context) {
	var req AccountLivenessCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if h.accountLivenessTestRunner == nil {
		response.Error(c, http.StatusServiceUnavailable, "Account test service unavailable")
		return
	}

	accounts, err := h.resolveLivenessAccounts(c.Request.Context(), req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if len(accounts) == 0 {
		response.BadRequest(c, "no accounts matched")
		return
	}

	concurrency := normalizeAccountLivenessConcurrency(req.Concurrency)
	jobs := make(chan *service.Account)
	results := make(chan AccountLivenessCheckItem, len(accounts))

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for account := range jobs {
				results <- h.runAccountLivenessCheck(c.Request.Context(), account)
			}
		}()
	}

	for _, account := range accounts {
		jobs <- account
	}
	close(jobs)
	wg.Wait()
	close(results)

	payload := AccountLivenessCheckResponse{
		Total:          len(accounts),
		ByPlatform:     map[string]AccountLivenessPlatformStats{},
		FailureReasons: map[string]int{},
		Items:          make([]AccountLivenessCheckItem, 0, len(accounts)),
	}
	var latencyTotal int64
	for item := range results {
		payload.Completed++
		payload.Items = append(payload.Items, item)
		updateAccountLivenessPlatformStats(payload.ByPlatform, item.Platform, item.Result)
		switch item.Result {
		case "success":
			payload.Success++
			latencyTotal += item.LatencyMs
		case "failed":
			payload.Failed++
			payload.FailureReasons[classifyAccountLivenessFailure(item.Message)]++
		case "skipped":
			payload.Skipped++
		}
	}
	if payload.Success > 0 {
		payload.AverageLatencyMs = latencyTotal / int64(payload.Success)
	}

	response.Success(c, payload)
}
```

确保 imports 包含 `sync` 和 `net/http`。如果 `net/http` 已存在，不重复添加。

- [ ] **步骤 6：实现单账号检测和状态回写**

在 `account_handler.go` 中新增方法：

```go
func (h *AccountHandler) runAccountLivenessCheck(ctx context.Context, account *service.Account) AccountLivenessCheckItem {
	item := AccountLivenessCheckItem{
		AccountID:    account.ID,
		AccountName:  account.Name,
		Platform:     account.Platform,
		Type:         account.Type,
		StatusBefore: account.Status,
		StatusAfter:  account.Status,
	}
	if account.Status == "inactive" {
		item.Result = "skipped"
		item.Message = "账号已禁用，跳过检测"
		return item
	}

	result, err := h.accountLivenessTestRunner.RunTestBackground(ctx, account.ID, "")
	if err != nil {
		item.Result = "failed"
		item.Message = sanitizeAccountLivenessMessage(err.Error())
	} else if result == nil {
		item.Result = "skipped"
		item.Message = "没有检测结果"
	} else if result.Status == "success" {
		item.Result = "success"
		item.LatencyMs = result.LatencyMs
		item.Message = "检测成功"
		if updated, clearErr := h.adminService.ClearAccountError(ctx, account.ID); clearErr == nil && updated != nil {
			item.StatusAfter = updated.Status
		} else if clearErr != nil {
			item.Result = "failed"
			item.Message = sanitizeAccountLivenessMessage(clearErr.Error())
		}
	} else {
		item.Result = "failed"
		item.LatencyMs = result.LatencyMs
		item.Message = sanitizeAccountLivenessMessage(result.ErrorMessage)
		if item.Message == "检测失败" && result.ResponseText != "" {
			item.Message = sanitizeAccountLivenessMessage(result.ResponseText)
		}
	}

	if item.Result == "failed" {
		if err := h.adminService.SetAccountError(ctx, account.ID, item.Message); err == nil {
			item.StatusAfter = service.StatusError
		} else {
			item.Message = sanitizeAccountLivenessMessage(err.Error())
		}
	}
	return item
}
```

- [ ] **步骤 7：注册路由**

在 `backend/internal/server/routes/admin.go` 的 `registerAccountRoutes` 中，把路由加入静态批量路由区域，必须在 `accounts.GET("/:id", ...)` 之前或至少在所有 `/:id` 动态路由之前：

```go
accounts.POST("/liveness-check", h.Admin.Account.LivenessCheck)
```

推荐调整开头为：

```go
accounts.GET("", h.Admin.Account.List)
accounts.POST("/liveness-check", h.Admin.Account.LivenessCheck)
accounts.POST("/check-mixed-channel", h.Admin.Account.CheckMixedChannel)
accounts.POST("/import/codex-session", h.Admin.Account.ImportCodexSession)
accounts.POST("/import/chatgpt-session", h.Admin.Account.ImportChatGPTSession)
accounts.POST("/sync/crs", h.Admin.Account.SyncFromCRS)
accounts.POST("/sync/crs/preview", h.Admin.Account.PreviewFromCRS)
accounts.GET("/:id", h.Admin.Account.GetByID)
```

保持其余路由不变。

- [ ] **步骤 8：运行后端测试验证通过**

运行：

```bash
D:/tool/go/bin/go test ./internal/handler/admin -run 'TestAccountHandlerLivenessCheck' -count=1
```

预期：PASS。

- [ ] **步骤 9：运行后端相关包测试**

运行：

```bash
D:/tool/go/bin/go test ./internal/handler/admin ./internal/server/routes -count=1
```

预期：PASS。

- [ ] **步骤 10：Commit**

```bash
git add backend/internal/handler/admin/account_handler.go backend/internal/handler/admin/account_liveness_check_test.go backend/internal/server/routes/admin.go
git commit -m "feat(账号管理): 添加批量存活检测接口"
```

---

## 任务 3：前端 API 类型与测试

**文件：**
- 修改：`frontend/src/api/admin/accounts.ts`
- 修改：`frontend/src/api/__tests__/admin.accounts.spec.ts`

- [ ] **步骤 1：编写失败的 API 测试**

在 `frontend/src/api/__tests__/admin.accounts.spec.ts` 的 `describe` 内新增：

```ts
  it('runs account liveness checks through the admin batch endpoint', async () => {
    const payload = {
      scope: 'selected' as const,
      account_ids: [1, 2],
      concurrency: 5
    }
    post.mockResolvedValueOnce({
      data: {
        total: 2,
        completed: 2,
        success: 1,
        failed: 1,
        skipped: 0,
        average_latency_ms: 120,
        by_platform: {},
        failure_reasons: {},
        items: []
      }
    })

    const result = await accountsAPI.livenessCheck(payload)

    expect(post).toHaveBeenCalledWith('/admin/accounts/liveness-check', payload, { timeout: 180000 })
    expect(result.total).toBe(2)
    expect(result.success).toBe(1)
  })
```

- [ ] **步骤 2：运行测试验证失败**

运行：

```bash
cd frontend && npm run test -- src/api/__tests__/admin.accounts.spec.ts --runInBand
```

如果该项目 Vitest 不支持 `--runInBand`，运行：

```bash
cd frontend && npm run test -- src/api/__tests__/admin.accounts.spec.ts
```

预期：FAIL，报错 `livenessCheck is not a function` 或类型缺失。

- [ ] **步骤 3：实现 API 类型和方法**

在 `frontend/src/api/admin/accounts.ts` 中 `BatchTodayStatsResponse` 附近新增：

```ts
export type AccountLivenessCheckScope = 'selected' | 'filtered'
export type AccountLivenessCheckResult = 'success' | 'failed' | 'skipped'

export interface AccountLivenessCheckFilters {
  platform?: string
  type?: string
  status?: string
  group?: string
  search?: string
  privacy_mode?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export interface AccountLivenessCheckRequest {
  scope: AccountLivenessCheckScope
  account_ids?: number[]
  filters?: AccountLivenessCheckFilters
  concurrency?: number
}

export interface AccountLivenessPlatformStats {
  success: number
  failed: number
  skipped: number
}

export interface AccountLivenessCheckItem {
  account_id: number
  account_name: string
  platform: string
  type: string
  result: AccountLivenessCheckResult
  latency_ms: number
  status_before: string
  status_after: string
  message: string
}

export interface AccountLivenessCheckResponse {
  total: number
  completed: number
  success: number
  failed: number
  skipped: number
  average_latency_ms: number
  by_platform: Record<string, AccountLivenessPlatformStats>
  failure_reasons: Record<string, number>
  items: AccountLivenessCheckItem[]
}
```

在 `getBatchTodayStats` 后新增：

```ts
export async function livenessCheck(
  payload: AccountLivenessCheckRequest
): Promise<AccountLivenessCheckResponse> {
  const { data } = await apiClient.post<AccountLivenessCheckResponse>(
    '/admin/accounts/liveness-check',
    payload,
    { timeout: 180000 }
  )
  return data
}
```

在 `accountsAPI` 对象里加入：

```ts
livenessCheck,
```

放在 `getBatchTodayStats` 后面。

- [ ] **步骤 4：运行 API 测试验证通过**

运行：

```bash
cd frontend && npm run test -- src/api/__tests__/admin.accounts.spec.ts
```

预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add frontend/src/api/admin/accounts.ts frontend/src/api/__tests__/admin.accounts.spec.ts
git commit -m "feat(账号管理): 添加存活检测前端接口"
```

---

## 任务 4：存活检测弹窗组件测试与实现

**文件：**
- 创建：`frontend/src/components/admin/account/AccountLivenessCheckModal.vue`
- 创建：`frontend/src/components/admin/account/__tests__/AccountLivenessCheckModal.spec.ts`

- [ ] **步骤 1：编写失败的组件测试**

创建 `frontend/src/components/admin/account/__tests__/AccountLivenessCheckModal.spec.ts`：

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountLivenessCheckModal from '../AccountLivenessCheckModal.vue'

const { livenessCheck } = vi.hoisted(() => ({
  livenessCheck: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      livenessCheck
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key
    })
  }
})

const BaseDialogStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: '<div v-if="show" data-test="dialog"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>'
}

const mountModal = (props = {}) => mount(AccountLivenessCheckModal, {
  props: {
    show: true,
    selectedIds: [],
    filters: { platform: '', type: '', status: '', group: '', search: '', privacy_mode: '', sort_by: 'name', sort_order: 'asc' },
    filteredCount: 12,
    ...props
  },
  global: {
    stubs: {
      BaseDialog: BaseDialogStub,
      Icon: true,
      LoadingSpinner: true
    }
  }
})

describe('AccountLivenessCheckModal', () => {
  beforeEach(() => {
    livenessCheck.mockReset()
  })

  it('shows filtered scope when there are no selected accounts', () => {
    const wrapper = mountModal()

    expect(wrapper.text()).toContain('admin.accounts.liveness.scopeFiltered')
    expect(wrapper.text()).toContain('12')
  })

  it('uses selected scope when selected accounts exist', async () => {
    livenessCheck.mockResolvedValueOnce({
      total: 2,
      completed: 2,
      success: 1,
      failed: 1,
      skipped: 0,
      average_latency_ms: 456,
      by_platform: { anthropic: { success: 1, failed: 0, skipped: 0 } },
      failure_reasons: { auth: 1 },
      items: [
        { account_id: 1, account_name: 'ok', platform: 'anthropic', type: 'oauth', result: 'success', latency_ms: 456, status_before: 'error', status_after: 'active', message: '检测成功' },
        { account_id: 2, account_name: 'bad', platform: 'openai', type: 'oauth', result: 'failed', latency_ms: 0, status_before: 'active', status_after: 'error', message: '401 unauthorized' }
      ]
    })
    const wrapper = mountModal({ selectedIds: [1, 2], filteredCount: 99 })

    await wrapper.get('[data-test="start-liveness-check"]').trigger('click')
    await flushPromises()

    expect(livenessCheck).toHaveBeenCalledWith({
      scope: 'selected',
      account_ids: [1, 2],
      concurrency: 5
    })
    expect(wrapper.text()).toContain('1')
    expect(wrapper.text()).toContain('456ms')
    expect(wrapper.text()).toContain('bad')
    expect(wrapper.text()).toContain('401 unauthorized')
  })

  it('emits completed when user finishes after a successful check', async () => {
    livenessCheck.mockResolvedValueOnce({
      total: 1,
      completed: 1,
      success: 1,
      failed: 0,
      skipped: 0,
      average_latency_ms: 100,
      by_platform: {},
      failure_reasons: {},
      items: []
    })
    const wrapper = mountModal({ selectedIds: [1] })

    await wrapper.get('[data-test="start-liveness-check"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="finish-liveness-check"]').trigger('click')

    expect(wrapper.emitted('completed')).toHaveLength(1)
  })

  it('shows request failure and allows retry', async () => {
    livenessCheck.mockRejectedValueOnce(new Error('network failed'))
    const wrapper = mountModal({ selectedIds: [1] })

    await wrapper.get('[data-test="start-liveness-check"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('network failed')
    expect(wrapper.get('[data-test="start-liveness-check"]').text()).toContain('admin.accounts.liveness.retry')
  })
})
```

- [ ] **步骤 2：运行组件测试验证失败**

运行：

```bash
cd frontend && npm run test -- src/components/admin/account/__tests__/AccountLivenessCheckModal.spec.ts
```

预期：FAIL，报错无法找到组件文件。

- [ ] **步骤 3：创建弹窗组件模板**

创建 `frontend/src/components/admin/account/AccountLivenessCheckModal.vue`，写入：

```vue
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
          <Icon v-if="checking" name="refresh" size="sm" class="animate-spin" />
          <Icon v-else name="play" size="sm" />
          {{ checking ? t('admin.accounts.liveness.checking') : result ? t('admin.accounts.liveness.retry') : t('admin.accounts.liveness.start') }}
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
  } catch (error: any) {
    errorMessage.value = error?.message || String(error)
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
```

- [ ] **步骤 4：运行组件测试验证通过或修复类型问题**

运行：

```bash
cd frontend && npm run test -- src/components/admin/account/__tests__/AccountLivenessCheckModal.spec.ts
```

预期：PASS。若 TypeScript 报 `MetricCard` 模板识别问题，将 `MetricCard` 提取为独立局部组件变量名保持 PascalCase，或改为模板内重复卡片 DOM；不要引入新依赖。

- [ ] **步骤 5：Commit**

```bash
git add frontend/src/components/admin/account/AccountLivenessCheckModal.vue frontend/src/components/admin/account/__tests__/AccountLivenessCheckModal.spec.ts
git commit -m "feat(账号管理): 添加存活检测弹窗"
```

---

## 任务 5：账号页入口集成与测试

**文件：**
- 修改：`frontend/src/views/admin/AccountsView.vue`
- 修改：`frontend/src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts`

- [ ] **步骤 1：扩展账号页测试 mock**

在 `frontend/src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts` 的 hoisted mock 里增加：

```ts
livenessCheck: vi.fn(),
```

在 `adminAPI.accounts` mock 里增加：

```ts
livenessCheck,
```

在 `beforeEach` 里增加：

```ts
livenessCheck.mockReset()
livenessCheck.mockResolvedValue({
  total: 0,
  completed: 0,
  success: 0,
  failed: 0,
  skipped: 0,
  average_latency_ms: 0,
  by_platform: {},
  failure_reasons: {},
  items: []
})
```

- [ ] **步骤 2：编写失败的入口测试**

在同一测试文件新增：

```ts
  it('opens account liveness check modal from more actions and reloads after completion', async () => {
    listAccounts.mockResolvedValueOnce({
      items: [],
      total: 12,
      page: 1,
      page_size: 20,
      pages: 1
    })
    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountBulkActionsBar: AccountBulkActionsBarStub,
          AccountActionMenu: AccountActionMenuStub,
          ImportChatGPTSessionModal: ImportChatGPTSessionModalStub,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          AccountLivenessCheckModal: {
            props: ['show', 'filteredCount'],
            emits: ['completed'],
            template: '<div data-test="liveness-modal" :data-show="String(show)" :data-filtered-count="String(filteredCount)"><button data-test="liveness-complete" @click="$emit(\'completed\')">complete</button></div>'
          },
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: BulkEditAccountModalStub,
          PlatformTypeBadge: true,
          AccountCapacityCell: true,
          AccountStatusIndicator: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountUsageCell: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    await wrapper.get('button[title="admin.accounts.moreActions"]').trigger('click')
    await wrapper.get('[data-test="open-liveness-check"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="liveness-modal"]').attributes('data-show')).toBe('true')
    expect(wrapper.get('[data-test="liveness-modal"]').attributes('data-filtered-count')).toBe('12')

    const callsBeforeComplete = listAccounts.mock.calls.length
    await wrapper.get('[data-test="liveness-complete"]').trigger('click')
    await flushPromises()

    expect(listAccounts.mock.calls.length).toBeGreaterThan(callsBeforeComplete)
  })
```

- [ ] **步骤 3：运行测试验证失败**

运行：

```bash
cd frontend && npm run test -- src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts
```

预期：FAIL，找不到 `AccountLivenessCheckModal` 或 `[data-test="open-liveness-check"]`。

- [ ] **步骤 4：在账号页导入并挂载弹窗**

修改 `frontend/src/views/admin/AccountsView.vue` imports：

```ts
import AccountLivenessCheckModal from '@/components/admin/account/AccountLivenessCheckModal.vue'
```

在状态区新增：

```ts
const showLivenessCheck = ref(false)
```

在 `isAnyModalOpen` computed 中加入：

```ts
showLivenessCheck.value ||
```

新增打开/关闭/完成方法：

```ts
const openLivenessCheck = () => {
  closeAccountToolsDropdown()
  showLivenessCheck.value = true
}

const closeLivenessCheck = () => {
  showLivenessCheck.value = false
}

const handleLivenessCheckCompleted = async () => {
  showLivenessCheck.value = false
  await reload()
  usageManualRefreshToken.value += 1
}
```

新增 computed，用于剥离不支持的筛选字段：

```ts
const livenessFilters = computed(() => ({
  platform: params.platform || '',
  type: params.type || '',
  status: normalizeAccountStatusFilter(params.status || ''),
  group: params.group || '',
  search: params.search || '',
  privacy_mode: params.privacy_mode || '',
  sort_by: sortState.sort_by,
  sort_order: sortState.sort_order
}))
```

- [ ] **步骤 5：在更多工具中加入按钮**

在 `AccountTableActions` 的 More Tools Dropdown 中 `toolActions` 区域内，在错误透传按钮前插入：

```vue
<button data-test="open-liveness-check" class="account-tools-menu-item" @click="openLivenessCheck">
  <span class="account-tools-menu-icon bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-300">
    <Icon name="shield" size="sm" />
  </span>
  <span class="flex-1 text-left">{{ t('admin.accounts.liveness.entry') }}</span>
</button>
```

- [ ] **步骤 6：在模板底部挂载弹窗**

在 `TLSFingerprintProfilesModal` 附近加入：

```vue
<AccountLivenessCheckModal
  :show="showLivenessCheck"
  :selected-ids="selIds"
  :filters="livenessFilters"
  :filtered-count="pagination.total"
  @close="closeLivenessCheck"
  @completed="handleLivenessCheckCompleted"
/>
```

- [ ] **步骤 7：运行账号页测试验证通过**

运行：

```bash
cd frontend && npm run test -- src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts
```

预期：PASS。

- [ ] **步骤 8：Commit**

```bash
git add frontend/src/views/admin/AccountsView.vue frontend/src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts
git commit -m "feat(账号管理): 接入存活检测入口"
```

---

## 任务 6：i18n 文案

**文件：**
- 修改：`frontend/src/i18n/locales/zh.ts`
- 修改：`frontend/src/i18n/locales/en.ts`

- [ ] **步骤 1：添加中文文案**

在 `frontend/src/i18n/locales/zh.ts` 的 `admin.accounts` 对象内新增：

```ts
liveness: {
  entry: '账号存活检测',
  title: '账号存活检测',
  scopeSelected: '将检测选中的 {count} 个账号',
  scopeFiltered: '将检测当前筛选的 {count} 个账号',
  executionHint: '后端会按并发 {concurrency} 执行真实轻量请求，检测完成后更新账号状态',
  start: '开始检测',
  retry: '重新检测',
  checking: '检测中...',
  progress: '检测进度',
  progressHint: '等待开始或查看完成结果',
  alive: '存活账号',
  failed: '异常账号',
  failedHint: '失败账号将更新为 error',
  avgLatency: '平均延迟',
  avgLatencyHint: '成功账号平均耗时',
  successRate: '成功率 {rate}%',
  byPlatform: '平台检测结果',
  platformSummary: '成功 {success} / 失败 {failed} / 跳过 {skipped}',
  failureReasons: '失败原因',
  noData: '暂无数据',
  noFailures: '暂无失败账号',
  details: '检测明细',
  account: '账号',
  platform: '平台',
  result: '结果',
  latency: '延迟',
  statusUpdate: '状态更新',
  message: '消息',
  waiting: '检测执行中，请稍候...',
  notStarted: '点击开始检测后显示明细',
  finishAndRefresh: '完成并刷新',
  failureReason: {
    auth: '认证失败',
    rate_limit: '限流或额度',
    timeout: '超时',
    other: '其他错误'
  },
  resultValue: {
    success: '成功',
    failed: '失败',
    skipped: '跳过'
  }
}
```

如果 `admin.accounts` 内已有 `liveness` 键，合并而不是覆盖。

- [ ] **步骤 2：添加英文文案**

在 `frontend/src/i18n/locales/en.ts` 的 `admin.accounts` 对象内新增：

```ts
liveness: {
  entry: 'Account Liveness Check',
  title: 'Account Liveness Check',
  scopeSelected: 'Checking {count} selected accounts',
  scopeFiltered: 'Checking {count} accounts in the current filter',
  executionHint: 'The backend runs real lightweight probes with concurrency {concurrency} and updates account status after completion',
  start: 'Start Check',
  retry: 'Run Again',
  checking: 'Checking...',
  progress: 'Progress',
  progressHint: 'Start a check or review completed results',
  alive: 'Alive Accounts',
  failed: 'Failed Accounts',
  failedHint: 'Failed accounts will be updated to error',
  avgLatency: 'Avg Latency',
  avgLatencyHint: 'Average latency of successful accounts',
  successRate: '{rate}% success rate',
  byPlatform: 'Results by Platform',
  platformSummary: 'Success {success} / Failed {failed} / Skipped {skipped}',
  failureReasons: 'Failure Reasons',
  noData: 'No data yet',
  noFailures: 'No failed accounts',
  details: 'Check Details',
  account: 'Account',
  platform: 'Platform',
  result: 'Result',
  latency: 'Latency',
  statusUpdate: 'Status Update',
  message: 'Message',
  waiting: 'Check is running...',
  notStarted: 'Details will appear after the check starts',
  finishAndRefresh: 'Finish and Refresh',
  failureReason: {
    auth: 'Authentication',
    rate_limit: 'Rate limit or quota',
    timeout: 'Timeout',
    other: 'Other'
  },
  resultValue: {
    success: 'Success',
    failed: 'Failed',
    skipped: 'Skipped'
  }
}
```

- [ ] **步骤 3：运行前端相关测试**

运行：

```bash
cd frontend && npm run test -- src/components/admin/account/__tests__/AccountLivenessCheckModal.spec.ts src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts src/api/__tests__/admin.accounts.spec.ts
```

预期：PASS。

- [ ] **步骤 4：Commit**

```bash
git add frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat(账号管理): 添加存活检测文案"
```

---

## 任务 7：全量验证与手工 UI 验证

**文件：**
- 不新增文件；运行验证命令。

- [ ] **步骤 1：运行后端定向测试**

```bash
D:/tool/go/bin/go test ./internal/handler/admin ./internal/server/routes -count=1
```

预期：PASS。

- [ ] **步骤 2：运行前端定向测试**

```bash
cd frontend && npm run test -- src/api/__tests__/admin.accounts.spec.ts src/components/admin/account/__tests__/AccountLivenessCheckModal.spec.ts src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts
```

预期：PASS。

- [ ] **步骤 3：运行前端类型检查或构建**

先查看 `frontend/package.json` 的 scripts；如果存在 `type-check`，运行：

```bash
cd frontend && npm run type-check
```

否则运行：

```bash
cd frontend && npm run build
```

预期：PASS。

- [ ] **步骤 4：运行后端包测试**

```bash
D:/tool/go/bin/go test ./...
```

预期：PASS。若集成测试需要外部服务导致失败，只记录失败包和原因，不跳过与本功能相关的 handler/service 测试。

- [ ] **步骤 5：启动开发环境并用浏览器验证 UI**

根据项目现有脚本启动后端和前端。若不确定命令，先读取根目录和 `frontend/package.json` 的 scripts。

在浏览器验证：

1. 登录管理员后台。
2. 打开账号管理页。
3. 点击更多操作中的“账号存活检测”。
4. 无选中账号时，确认弹窗显示当前筛选数量。
5. 选中 1-2 个账号后再次打开，确认弹窗显示选中数量。
6. 点击“开始检测”，确认按钮进入检测中，完成后出现 4 张指标卡片、平台结果、失败原因和明细。
7. 点击“完成并刷新”，确认弹窗关闭且账号列表刷新，失败账号状态为 `error`，成功账号状态恢复为 `active`。

- [ ] **步骤 6：最终 Commit**

如果验证修复产生额外改动：

```bash
git add <changed-files>
git commit -m "fix(账号管理): 完善存活检测验证问题"
```

如果没有额外改动，不创建空 commit。

---

## 规格覆盖自检

- 入口：任务 5 覆盖账号管理页“账号存活检测”入口。
- 弹窗仪表盘：任务 4 覆盖检测控制区、4 张指标卡片、平台分布、失败原因、明细表和完成按钮。
- 真实批量检测：任务 2 复用 `AccountTestService.RunTestBackground`，后端控制并发。
- 状态回写：任务 2 使用 `ClearAccountError` 和 `SetAccountError`，任务 1 覆盖成功/失败回写测试。
- 当前筛选/选中范围：任务 1 和任务 5 覆盖 selected / filtered。
- 前端 API、i18n、测试和手工验证：任务 3、6、7 覆盖。

## 占位符扫描

本计划没有未定义的 TODO/TBD；低优先级导出失败列表未纳入首版实现，符合规格中的“不阻塞首版”。
