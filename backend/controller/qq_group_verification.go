package controller

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GenerateQQGroupVerificationCode 供 AstrBot 插件（QQ 群机器人）调用：
// 请求头 X-QQ-Bot-Secret 与系统设置的 Bot 密钥做常数时间比对，通过后为指定 QQ 签发验证码。
func GenerateQQGroupVerificationCode(c *gin.Context) {
	secret := model.QQGroupVerifySecret()
	if secret == "" {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": model.ErrQQCodeNotEnabled.Error()})
		return
	}
	provided := c.GetHeader("X-QQ-Bot-Secret")
	if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "无效的 Bot 密钥"})
		return
	}
	var req struct {
		QQId string `json:"qq_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请求格式错误"})
		return
	}
	req.QQId = strings.TrimSpace(req.QQId)
	code, err := model.CreateQQVerificationCode(req.QQId)
	if err != nil {
		status := http.StatusBadRequest
		if err == model.ErrQQCodeRateLimit {
			status = http.StatusTooManyRequests
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"code":       code,
			"expires_in": model.QQGroupCodeExpiresInSec,
		},
	})
}

// BindQQGroup 当前登录用户提交在群里获取的验证码，完成 QQ 群验证绑定。
func BindQQGroup(c *gin.Context) {
	userId := c.GetInt("id")
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "请求格式错误")
		return
	}
	qqId, err := model.BindQQGroupCode(strings.TrimSpace(req.Code), userId)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"qq_id": qqId})
}

// GetQQGroupInfo 当前用户视角的 QQ 群验证状态：开关、群号列表、自己的绑定情况。
// 供个人资料的绑定弹窗展示群号与真实倒计时（按该用户QQ当前有效验证码的过期时间）。
func GetQQGroupInfo(c *gin.Context) {
	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var qqVerifiedAt int64
	if user.QQVerifiedAt != nil {
		qqVerifiedAt = *user.QQVerifiedAt
	}
	data := gin.H{
		"enabled":        model.QQGroupVerificationEnabled(),
		"groups":         model.QQGroupNumbers(),
		"code_ttl":       model.QQGroupCodeExpiresInSec,
		"resend_wait":    int(model.QQGroupCodeResendWait.Seconds()),
		"qq_id":          user.QQId,
		"qq_verified_at": qqVerifiedAt,
		"server_time":    time.Now().Unix(),
		"created_at":     user.CreatedAt,
	}
	// 邮箱与QQ一致时，才能确定该用户对应的 QQ 号，从而查它当前有效验证码的过期时间。
	if emailQQ, emailErr := model.QQNumberFromEmail(user.Email); emailErr == nil {
		if expiresAt, ok := model.LatestActiveQQCodeExpiry(emailQQ); ok {
			data["code_expires_at"] = expiresAt
		}
	}
	common.ApiSuccess(c, data)
}

type qqVerificationUserItem struct {
	Id           int    `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Role         int    `json:"role"`
	Status       int    `json:"status"`
	Email        string `json:"email"`
	Group        string `json:"group"`
	QQId         string `json:"qq_id"`
	QQVerifiedAt int64  `json:"qq_verified_at"`
	CreatedAt    int64  `json:"created_at"`
}

// GetQQVerificationUsers 管理端：按用户列出 QQ 群验证状态，支持关键词与已验证/未验证筛选。
func GetQQVerificationUsers(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := strings.TrimSpace(c.Query("keyword"))
	verified := -1
	if c.Query("verified") != "" {
		verified, _ = strconv.Atoi(c.Query("verified"))
	}
	users, total, err := model.GetQQVerificationUsers(keyword, verified, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]qqVerificationUserItem, 0, len(users))
	for _, u := range users {
		item := qqVerificationUserItem{
			Id:          u.Id,
			Username:    u.Username,
			DisplayName: u.DisplayName,
			Role:        u.Role,
			Status:      u.Status,
			Email:       u.Email,
			Group:       u.Group,
			QQId:        u.QQId,
			CreatedAt:   u.CreatedAt,
		}
		if u.QQVerifiedAt != nil {
			item.QQVerifiedAt = *u.QQVerifiedAt
		}
		items = append(items, item)
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// UnbindUserQQ 管理端：解除某用户的 QQ 群绑定，其下次调用模型需重新验证。
func UnbindUserQQ(c *gin.Context) {
	var req struct {
		UserId int `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UserId <= 0 {
		common.ApiErrorMsg(c, "无效的用户 ID")
		return
	}
	target, err := model.GetUserById(req.UserId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if target.Role >= common.RoleAdminUser && c.GetInt("role") < common.RoleRootUser {
		common.ApiErrorMsg(c, "无权操作管理员账号")
		return
	}
	if err := model.UnbindUserQQ(req.UserId); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
