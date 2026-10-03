package model

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	QQGroupCodeTTL          = 10 * time.Minute
	QQGroupCodeResendWait   = 60 * time.Second
	QQGroupCodeMaxAttempts  = 5
	QQGroupCodeExpiresInSec = 600
)

var (
	ErrQQCodeInvalid       = errors.New("验证码错误或已过期")
	ErrQQCodeRateLimit     = errors.New("获取过于频繁，请稍后再试")
	ErrQQCodeQQTaken       = errors.New("该QQ号已绑定其他账号，无法完成绑定")
	ErrQQCodeNotEnabled    = errors.New("QQ群验证未启用：请先配置 Bot 密钥")
	ErrQQCodeEmailInvalid  = errors.New("请先在个人资料中绑定与QQ号一致的QQ邮箱，再进行QQ群验证")
	ErrQQCodeEmailMismatch = errors.New("验证码对应的QQ号与您账号邮箱的QQ号不一致，无法通过QQ群验证")
	ErrQQCodeInvalidQQId   = errors.New("无效的 QQ 号")
)

// QQVerificationCode 记录群里生成的验证码。code 本身只以 HMAC 摘要落库，
// 校验时按摘要精确查找，避免遍历比对。
type QQVerificationCode struct {
	Id             int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	CodeHash       string `json:"-" gorm:"type:varchar(64);column:code_hash;index"`
	QQId           string `json:"qq_id" gorm:"type:varchar(128);column:qq_id;index"`
	CreatedAt      int64  `json:"created_at" gorm:"column:created_at"`
	ExpiresAt      int64  `json:"expires_at" gorm:"column:expires_at"`
	UsedAt         *int64 `json:"used_at" gorm:"column:used_at"`
	FailedAttempts int    `json:"failed_attempts" gorm:"column:failed_attempts"`
	UserId         int    `json:"user_id" gorm:"column:user_id"`
}

// QQGroupVerifySecret 返回用于鉴权 AstrBot 插件并给验证码做摘要的共享密钥：
// 优先取系统设置 QQBotSecret（带 Secret 后缀的键不会泄露给前端），回退到环境变量。
func QQGroupVerifySecret() string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	if v, ok := common.OptionMap["QQBotSecret"]; ok && v != "" {
		return v
	}
	return os.Getenv("QQ_BOT_SECRET")
}

func qqCodeHash(secret, code string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

func generateQQCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func isValidQQId(s string) bool {
	if len(s) < 5 || len(s) > 12 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// CreateQQVerificationCode 为某个QQ标识签发验证码：
// 同一QQ 60 秒内只能获取一个，新码会使旧码作废；有效期 10 分钟。
func CreateQQVerificationCode(qqId string) (string, error) {
	secret := QQGroupVerifySecret()
	if secret == "" {
		return "", ErrQQCodeNotEnabled
	}
	if !isValidQQId(qqId) {
		return "", ErrQQCodeInvalidQQId
	}
	now := time.Now().Unix()

	var recent QQVerificationCode
	err := DB.Where("qq_id = ? AND used_at IS NULL AND expires_at > ?", qqId, now).
		Order("id desc").First(&recent).Error
	if err == nil && recent.CreatedAt+int64(QQGroupCodeResendWait.Seconds()) > now {
		return "", ErrQQCodeRateLimit
	}

	// 使该QQ此前未使用的码全部作废
	DB.Model(&QQVerificationCode{}).
		Where("qq_id = ? AND used_at IS NULL", qqId).
		Update("used_at", now)

	code, err := generateQQCode()
	if err != nil {
		return "", err
	}
	row := &QQVerificationCode{
		CodeHash:  qqCodeHash(secret, code),
		QQId:      qqId,
		CreatedAt: now,
		ExpiresAt: now + int64(QQGroupCodeTTL.Seconds()),
	}
	if err := DB.Create(row).Error; err != nil {
		return "", err
	}
	return code, nil
}

// BindQQGroupCode 校验验证码并把 QQ 标识绑定到当前用户：
// 错误尝试计数累加，超过上限后该码作废；一个 QQ 只能绑定一个账号。
func BindQQGroupCode(code string, userId int) (string, error) {
	secret := QQGroupVerifySecret()
	if secret == "" {
		return "", ErrQQCodeNotEnabled
	}
	now := time.Now().Unix()

	// 先取用户及其邮箱对应的 QQ 号：错误尝试要记到这个 QQ 名下。
	// （验证码按摘要查找，错误的码本身查不到记录，所以失败次数只能按提交者的 QQ 归集。）
	user, err := GetUserById(userId, false)
	if err != nil {
		return "", err
	}
	emailQQ, emailErr := QQNumberFromEmail(user.Email)

	// failAttempt 给该 QQ 名下所有仍有效的验证码累加一次失败次数。
	// 无法确定提交者 QQ（邮箱不是 QQ 邮箱）时不记。
	failAttempt := func() {
		if emailErr != nil || emailQQ == "" {
			return
		}
		DB.Model(&QQVerificationCode{}).
			Where("qq_id = ? AND used_at IS NULL AND expires_at > ?", emailQQ, now).
			UpdateColumn("failed_attempts", gorm.Expr("failed_attempts + 1"))
	}

	if !isNumeric6(code) {
		failAttempt()
		return "", ErrQQCodeInvalid
	}

	var row QQVerificationCode
	if err := DB.Where("code_hash = ?", qqCodeHash(secret, code)).
		Order("id desc").First(&row).Error; err != nil {
		failAttempt()
		return "", ErrQQCodeInvalid
	}
	if row.UsedAt != nil || row.ExpiresAt < now {
		failAttempt()
		return "", ErrQQCodeInvalid
	}
	if row.FailedAttempts >= QQGroupCodeMaxAttempts {
		return "", ErrQQCodeInvalid
	}

	// 校验账号邮箱与验证码对应的 QQ 号一致。
	// 平台用户均使用 QQ 邮箱（@qq.com / @vip.qq.com），邮箱前缀即 QQ 号。
	// 不一致时不消耗验证码也不计失败次数，用户修正邮箱后仍可用原码重试。
	if emailErr != nil {
		return "", ErrQQCodeEmailInvalid
	}
	if emailQQ != row.QQId {
		return "", ErrQQCodeEmailMismatch
	}

	var count int64
	if err := DB.Model(&User{}).
		Where("qq_id = ? AND id <> ?", row.QQId, userId).
		Count(&count).Error; err != nil {
		return "", err
	}
	if count > 0 {
		// 该码对应的 QQ 已被占用，直接作废防止继续被尝试
		DB.Model(&row).Update("used_at", now)
		return "", ErrQQCodeQQTaken
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&QQVerificationCode{}).
			Where("id = ?", row.Id).
			Updates(map[string]any{"used_at": now, "user_id": userId}).Error; err != nil {
			return err
		}
		return tx.Model(&User{}).
			Where("id = ?", userId).
			Updates(map[string]any{"qq_id": row.QQId, "qq_verified_at": now}).Error
	})
	if err != nil {
		return "", err
	}
	invalidateUserCache(userId)
	return row.QQId, nil
}

func isNumeric6(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// LatestActiveQQCodeExpiry 返回该 QQ 当前未使用、未过期验证码的过期时间。
// 用于给绑定弹窗展示真实的剩余有效期；没有有效码时返回 false。
func LatestActiveQQCodeExpiry(qqId string) (int64, bool) {
	if qqId == "" {
		return 0, false
	}
	var row QQVerificationCode
	err := DB.Where("qq_id = ? AND used_at IS NULL AND expires_at > ?", qqId, time.Now().Unix()).
		Order("id desc").First(&row).Error
	if err != nil {
		return 0, false
	}
	return row.ExpiresAt, true
}

// QQGroupVerificationEnabled 读系统设置总开关（默认 false）。
func QQGroupVerificationEnabled() bool {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return common.OptionMap["QQGroupVerificationEnabled"] == "true"
}

// QQGroupNumbers 返回提醒里展示的群号列表。
// 支持在一个配置里填多个群号，用逗号（中英文）、空格或换行分隔。
func QQGroupNumbers() []string {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap["QQGroupNumber"]
	common.OptionMapRWMutex.RUnlock()
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ';' || r == '；'
	})
	result := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			result = append(result, f)
		}
	}
	return result
}

// QQGroupVerificationHint 返回未验证用户调用模型时被拦截的提示文案。
func QQGroupVerificationHint() string {
	groups := QQGroupNumbers()
	switch len(groups) {
	case 0:
		return "请加入QQ群并完成验证"
	case 1:
		return "请加入QQ群 " + groups[0] + " 并完成验证"
	default:
		return "请加入QQ群 " + strings.Join(groups, " 或 ") + " 并完成验证"
	}
}

// QQNumberFromEmail 从 QQ 邮箱地址中提取 QQ 号。
// 支持 @qq.com 与 @vip.qq.com，前缀必须是纯数字的 QQ 号。
func QQNumberFromEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	qqNumber := ""
	for _, domain := range []string{"@vip.qq.com", "@qq.com"} {
		if strings.HasSuffix(email, domain) {
			qqNumber = strings.TrimSuffix(email, domain)
			break
		}
	}
	if qqNumber == "" || len(qqNumber) < 5 || len(qqNumber) > 12 {
		return "", errors.New("不是有效的QQ邮箱")
	}
	for _, r := range qqNumber {
		if r < '0' || r > '9' {
			return "", errors.New("邮箱前缀不是QQ号")
		}
	}
	return qqNumber, nil
}

// GetQQVerificationUsers 管理端的QQ验证状态列表：
// keyword 模糊匹配用户名/邮箱/QQ号；verified 为 1 只看已验证、0 只看未验证、其他为全部。
func GetQQVerificationUsers(keyword string, verified int, startIdx int, num int) ([]*User, int64, error) {
	tx := DB.Model(&User{})
	if keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("username LIKE ? OR email LIKE ? OR qq_id LIKE ?", like, like, like)
	}
	switch verified {
	case 1:
		tx = tx.Where("qq_verified_at IS NOT NULL AND qq_verified_at > 0")
	case 0:
		tx = tx.Where("qq_verified_at IS NULL OR qq_verified_at = 0")
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []*User
	err := tx.Select("id", "username", "display_name", "role", "status", "email", "group", "qq_id", "qq_verified_at", "created_at").
		Order("qq_verified_at desc, id asc").
		Limit(num).Offset(startIdx).
		Find(&users).Error
	return users, total, err
}

// UnbindUserQQ 清除某用户的QQ群绑定（qq_id 与验证时间），下次调用模型需重新验证。
func UnbindUserQQ(userId int) error {
	if userId <= 0 {
		return errors.New("无效的用户 ID")
	}
	res := DB.Model(&User{}).
		Where("id = ?", userId).
		Updates(map[string]any{"qq_id": "", "qq_verified_at": 0})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("用户不存在")
	}
	return invalidateUserCache(userId)
}
