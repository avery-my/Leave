package schoollogin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func fetchCaptcha(ctx context.Context, client *http.Client, baseURL string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/captcha", nil)
	if err != nil {
		return "", "", fmt.Errorf("构建验证码请求: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("请求验证码: %w", err)
	}
	defer resp.Body.Close()
	if err := schoolHTTPError(resp.StatusCode, "获取验证码"); err != nil {
		return "", "", err
	}

	type captchaPayload struct {
		CID    string `json:"cid"`
		Base64 string `json:"base64"`
	}
	var result struct {
		captchaPayload
		Data *captchaPayload `json:"data"`
	}
	const maxCaptchaBytes = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCaptchaBytes+1))
	if err != nil {
		return "", "", fmt.Errorf("读取验证码响应: %w", err)
	}
	if len(body) > maxCaptchaBytes {
		return "", "", errors.New("验证码响应超过大小限制")
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", errors.New("验证码响应格式异常")
	}
	captcha := result.captchaPayload
	if result.Data != nil {
		captcha = *result.Data
	}
	captchaID, captchaBase64 := captcha.CID, captcha.Base64
	if captchaID == "" || captchaBase64 == "" {
		return "", "", errors.New("验证码响应缺少 cid 或 base64")
	}
	return captchaID, captchaBase64, nil
}
