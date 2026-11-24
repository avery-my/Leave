package schoollogin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"backend/internal/config"
	"backend/internal/utils/file"
)

type captchaRecognizer interface {
	RecognizeBase64(context.Context, string) (string, error)
}

type Client struct {
	baseURL     string
	storageRoot string
	password    string
	transport   http.RoundTripper
	ocr         captchaRecognizer
}

func schoolHTTPError(status int, operation string) error {
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusRequestTimeout, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return ErrUpstreamUnavailable
	default:
		return fmt.Errorf("%s失败 (HTTP %d)", operation, status)
	}
}

func NewClient(cfg config.SchoolConfig, storageRoot string, ocrService captchaRecognizer) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{
		baseURL:     cfg.URL,
		storageRoot: storageRoot,
		password:    cfg.Password,
		transport:   transport,
		ocr:         ocrService,
	}
}

// FetchStudentPhoto 保存指定学校账号的 JPEG 照片，返回相对于上传根目录的路径。
func (c *Client) FetchStudentPhoto(ctx context.Context, username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("请提供学校用户名")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if c.ocr == nil {
		return "", errors.New("未配置学校验证码识别服务")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", fmt.Errorf("初始化学校登录会话: %w", err)
	}
	client := &http.Client{
		Jar:       jar,
		Transport: c.transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	captchaID, captchaBase64, err := fetchCaptcha(ctx, client, c.baseURL)
	if err != nil {
		return "", err
	}

	captchaAnswer, err := c.ocr.RecognizeBase64(ctx, captchaBase64)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	if err != nil {
		return "", fmt.Errorf("识别学校验证码: %w", err)
	}
	captchaAnswer = strings.TrimSpace(captchaAnswer)
	if captchaAnswer == "" {
		return "", errors.New("学校验证码识别结果为空")
	}

	token, err := login(ctx, client, c.baseURL, username, c.password, captchaID, captchaAnswer)
	if err != nil {
		return "", err
	}
	photoID, err := fetchPhotoID(ctx, client, c.baseURL, token)
	if err != nil {
		return "", err
	}
	photo, err := downloadPhoto(ctx, client, c.baseURL, token, photoID)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	keys, err := file.Save(c.storageRoot, "photo", file.Input{
		Extension: ".jpg",
		Content:   bytes.NewReader(photo),
	})
	if err != nil {
		return "", fmt.Errorf("保存学校照片: %w", err)
	}
	return keys[0], nil
}
