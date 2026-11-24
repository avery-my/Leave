package schoollogin

import "errors"

var (
	ErrCaptchaRejected     = errors.New("学校验证码错误或已过期")
	ErrUpstreamUnavailable = errors.New("学校系统暂时不可用")

	errInvalidCredentials = errors.New("学校账号或密码错误")
	errLoginDecode        = errors.New("学校登录参数解码错误")
	errPhotoRequest       = errors.New("请求照片失败")
	errPhotoRead          = errors.New("读取照片失败")
)
