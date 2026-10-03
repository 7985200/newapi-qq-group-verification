package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// QQGroupGateApplies 判断本次 relay 请求是否要求先完成 QQ 群验证：
// 仅在总开关开启、路由标记为 relay（即模型调用类接口）、且调用者不是管理员时生效。
// 管理界面 /api/*、旧版 /v1/dashboard/billing 等非 relay 路由不受影响。
func QQGroupGateApplies(c *gin.Context, userRole int) bool {
	if c.GetString(RouteTagKey) != "relay" {
		return false
	}
	if !model.QQGroupVerificationEnabled() {
		return false
	}
	return userRole < common.RoleAdminUser
}
