package schoollogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
	CID      string `json:"cid"`
	Remember bool   `json:"remember"`
}

func login(ctx context.Context, client *http.Client, baseURL, username, password, cid, answer string) (string, error) {
	scriptURL := fmt.Sprintf("%s/api/sec_js?t=%d", baseURL, time.Now().UnixMilli())
	scriptRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, scriptURL, nil)
	if err != nil {
		return "", err
	}
	script, err := client.Do(scriptRequest)
	if err != nil {
		return "", fmt.Errorf("获取安全脚本: %w", err)
	}
	defer script.Body.Close()
	if err := schoolHTTPError(script.StatusCode, "获取安全脚本"); err != nil {
		return "", err
	}
	source, err := io.ReadAll(io.LimitReader(script.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("读取安全脚本: %w", err)
	}
	key, err := parseLoginKey(string(source))
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(loginRequest{
		Username: key.encrypt(username),
		Password: key.encrypt(password),
		Code:     key.encrypt(answer),
		CID:      key.encrypt(cid),
		Remember: false,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/login", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("提交登录: %w", err)
	}
	defer resp.Body.Close()
	statusErr := schoolHTTPError(resp.StatusCode, "登录")
	if statusErr == ErrUpstreamUnavailable {
		return "", statusErr
	}
	const maxResponseBytes = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("读取登录响应 (HTTP %d): %w", resp.StatusCode, err)
	}
	if len(body) > maxResponseBytes {
		return "", fmt.Errorf("登录响应超过大小限制 (HTTP %d)", resp.StatusCode)
	}

	var result struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
		Data    struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	decodeErr := json.Unmarshal(body, &result)
	if statusErr != nil || (decodeErr == nil && result.Code != nil && *result.Code != 0) {
		// 只显示可识别的错误原因。响应正文和其他字段可能包含会话信息。
		if decodeErr == nil {
			switch message := result.Message; {
			case strings.Contains(message, "参数解码错误"):
				return "", errLoginDecode
			case strings.Contains(message, "账号或密码错误"), strings.Contains(message, "用户名或密码错误"):
				return "", errInvalidCredentials
			case strings.Contains(message, "验证码已过期"), strings.Contains(message, "验证码错误"), strings.Contains(message, "验证码不正确"):
				return "", ErrCaptchaRejected
			}
			if result.Code != nil {
				return "", fmt.Errorf("登录失败 (HTTP %d，业务码 %d)", resp.StatusCode, *result.Code)
			}
		}
		return "", statusErr
	}
	if decodeErr != nil {
		return "", errors.New("登录接口响应格式异常")
	}
	if result.Code == nil {
		return "", errors.New("登录接口响应缺少业务状态码")
	}
	if result.Data.AccessToken == "" {
		return "", errors.New("登录响应缺少 data.access_token")
	}
	return result.Data.AccessToken, nil
}
