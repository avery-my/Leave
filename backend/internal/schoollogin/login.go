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
	if script.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取安全脚本失败 (HTTP %d)", script.StatusCode)
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
	if resp.StatusCode != http.StatusOK || (decodeErr == nil && result.Code != nil && *result.Code != 0) {
		status := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if decodeErr == nil && result.Code != nil {
			status += fmt.Sprintf("，业务码 %d", *result.Code)
		}
		// 只显示可识别的错误原因。响应正文和其他字段可能包含会话信息。
		if decodeErr == nil {
			for _, reason := range []string{"参数解码错误", "验证码已过期", "验证码错误", "验证码不正确", "账号或密码错误", "用户名或密码错误"} {
				if strings.Contains(result.Message, reason) {
					return "", fmt.Errorf("登录失败 (%s) %s", status, reason)
				}
			}
		}
		return "", fmt.Errorf("登录失败 (%s)", status)
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
