# NewAPI QQ 群验证补丁包

给 NewAPI 增加"必须加入 QQ 群完成验证才能调用模型"的能力，配套 AstrBot 插件（见同目录 `astrbot-plugin/` 与 `astrbot_plugin_newapi_qq_group_verify_v1.0.0.zip`）。

> 本补丁基于 **NewAPI v1.0.0-rc.36** 编写，其他版本大同小异，按说明粘贴即可。
> **只包含 QQ 群验证这一个功能**，不含头像上传、排行榜、日志开关等其它改动。

## 功能

- 用户在 QQ 群发送 `/获取验证码`，拿到 6 位验证码（10 分钟有效、60 秒冷却）；
- 在站点 **个人资料 → 绑定QQ群** 弹窗提交验证码完成绑定，弹窗显示**真实剩余有效期**；
- 强校验：**账号邮箱必须是该 QQ 号的 QQ 邮箱**（前缀 = QQ 号），且一个 QQ 只能绑一个账号；
- 未验证的普通用户调用 `/v1/*` 模型接口被拦截：`403 请加入QQ群 X 并完成验证`（管理员不受影响）；
- **多群支持**：群号填 `123456789, 987654321`，提示自动变成"请加入QQ群 123456789 **或** 987654321 并完成验证"；
- 后台「安全与限制 → QQ 群验证」：开关、群号、Bot 密钥、验证状态管理（搜索 / 已验证 / 未验证 / 解除绑定）。

## 目录结构

```
backend/   ← 3 个新增 Go 文件（按同名路径复制进你的 NewAPI）
frontend/  ← 2 个新增前端文件（按同名路径复制）
```

---

## 一、后端安装

### 1. 新增文件（直接复制）

| 文件 | 目标路径 |
|---|---|
| `backend/model/qq_group_verification.go` | `model/qq_group_verification.go` |
| `backend/middleware/qq_group_verification.go` | `middleware/qq_group_verification.go` |
| `backend/controller/qq_group_verification.go` | `controller/qq_group_verification.go` |

### 2. `model/user.go`

User 结构体加两个字段：

```go
QQId         string `json:"qq_id" gorm:"column:qq_id;index"`
QQVerifiedAt *int64 `json:"qq_verified_at" gorm:"column:qq_verified_at"`
```

`ToBaseUser()` 在 `return cache` 前加：

```go
if user.QQVerifiedAt != nil {
    cache.QQVerifiedAt = *user.QQVerifiedAt
}
```

`GetSelfUserById` 的 Select 列表里加 `"qq_id", "qq_verified_at"`。

### 3. `model/user_cache.go`

- `userCacheSchemaVersion` 从 `2` 改成 `3`；
- `UserBase` 结构体加：`QQVerifiedAt int64 \`json:"qq_verified_at"\``。

### 4. `model/main.go`

AutoMigrate 列表里加：

```go
&QQVerificationCode{},
```

### 5. `model/option.go`（InitOptionMap）

```go
common.OptionMap["QQGroupVerificationEnabled"] = "false"
common.OptionMap["QQGroupNumber"] = ""
common.OptionMap["QQBotSecret"] = ""
```

### 6. `middleware/auth.go`（TokenAuth 中间件）

找到 `userCache.WriteContext(c)` 这一行，**紧接着**加门控：

```go
if QQGroupGateApplies(c, userCache.Role) && userCache.QQVerifiedAt <= 0 {
    abortWithOpenAiMessage(c, http.StatusForbidden, model.QQGroupVerificationHint(), types.ErrorCode("qq_group_verification_required"))
    return
}
```

（`http` / `model` / `types` 一般已在该文件导入；`types` 是 `relaykit/types`。）

### 7. `controller/user.go`（buildSelfUserData）

把 `return map[string]any{` 改成 `data := map[string]any{`，在函数末尾加：

```go
qqVerifiedAt := int64(0)
if user.QQVerifiedAt != nil {
    qqVerifiedAt = *user.QQVerifiedAt
}
data["qq_id"] = user.QQId
data["qq_verified_at"] = qqVerifiedAt
return data
```

### 8. `router/api-router.go`

```go
// 公共：Bot 发码（AstrBot 插件调用）
apiRouter.POST("/qq-group/verification/code", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GenerateQQGroupVerificationCode)

// 登录用户：状态查询 + 绑定
selfRoute.GET("/qq-group/info", controller.GetQQGroupInfo)
selfRoute.POST("/qq-group/bind", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.BindQQGroup)

// 管理员：验证状态管理
qqAdminRoute := apiRouter.Group("/qq-group")
qqAdminRoute.Use(middleware.AdminAuth())
{
    qqAdminRoute.GET("/verifications", controller.GetQQVerificationUsers)
    qqAdminRoute.POST("/unbind", middleware.DisableCache(), controller.UnbindUserQQ)
}
```

### 9. 编译

```bash
go build -o new-api .
```

（SQLite 用户无需额外操作；MySQL/PG 会自动补列建表。）

---

## 二、前端安装

### 1. 新增文件（直接复制）

| 文件 | 目标路径 |
|---|---|
| `frontend/profile/qq-group-bind-dialog.tsx` | `web/src/features/profile/components/qq-group-bind-dialog.tsx` |
| `frontend/system-settings/qq-group-section.tsx` | `web/src/features/system-settings/security/qq-group-section.tsx` |

### 2. 小改动（在你自己的文件里对应位置添加）

- `features/profile/types.ts`：`UserProfile` 加 `qq_id?: string; qq_verified_at?: number;`，并加 `QQGroupInfo` 类型（见绑定弹窗里的引用）；
- `features/profile/api.ts`：加 `getQQGroupInfo()`（GET `/api/user/qq-group/info`）和 `bindQQGroup(code)`（POST `/api/user/qq-group/bind`）；
- `features/profile/components/profile-header.tsx`：邮箱一行后加"绑定QQ群"按钮（未验证时显示），并挂载 `QQGroupBindDialog`；
- `features/security/components/account-bindings.tsx`：绑定列表加"QQ群"一项，点击打开同一个弹窗；
- `features/system-settings/types.ts`：`SecuritySettings` 加 `QQGroupVerificationEnabled: boolean; QQGroupNumber: string;`；
- `features/system-settings/security/index.tsx`：默认值加 `QQGroupVerificationEnabled: false, QQGroupNumber: ''`；
- `features/system-settings/security/section-registry.tsx`：注册 `QQGroupSection`（参考其它 section 的 build 写法）。

### 3. 构建

```bash
cd web && bun install && bun run build
```

---

## 三、配置

后台：**系统设置 → 安全与限制 → QQ 群验证**

| 配置 | 说明 |
|---|---|
| 启用 QQ 群验证 | 总开关，开启后未验证用户调用模型被拦截 |
| QQ 群号 | 展示在拦截提示里；多个群号用逗号分隔 |
| Bot 密钥 | 与 AstrBot 插件一致，建议 `openssl rand -hex 24` 生成 |

## 四、AstrBot 插件

- **安装**：把 `astrbot-plugin/` 放/上传到 AstrBot 插件目录，或直接上传 `astrbot_plugin_newapi_qq_group_verify_v1.0.0.zip`；
- **配置**：`newapi_base_url`（你的站点地址）、`bot_secret`（与后台 Bot 密钥一致）、`allowed_groups`（允许的群号，留空=所有群）；
- 把机器人拉进群，群成员发 `/获取验证码` 即可。

## 注意事项

- 邮箱与 QQ 强绑定是本方案的核心（也是防小号的关键）：**非 QQ 邮箱用户无法通过验证**，如果你的站允许其他邮箱注册，请自行调整 `model/qq_group_verification.go` 里的 `QQNumberFromEmail`；
- 站点需为 HTTPS 且 AstrBot 能访问到（插件服务端调用）；
- 验证码只以 HMAC 摘要落库，明文不存储。
