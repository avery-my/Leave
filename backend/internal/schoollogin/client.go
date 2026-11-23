package schoollogin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"backend/internal/config"
	"backend/internal/utils/file"
)

type Client struct {
	baseURL     string
	storageRoot string
	password    string
	transport   http.RoundTripper
}

func NewClient(cfg config.SchoolConfig, storageRoot string) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if baseURL == "" {
		return nil, errors.New("请配置 SCHOOL_URL")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Path != "" || parsed.ForceQuery || parsed.RawQuery != "" ||
		strings.Contains(baseURL, "#") {
		return nil, errors.New("SCHOOL_URL 应为 HTTPS 网站根地址")
	}
	if cfg.Password == "" {
		return nil, errors.New("请配置 SCHOOL_PASSWORD")
	}

	if strings.TrimSpace(storageRoot) == "" {
		return nil, errors.New("请配置 storage.root_dir")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{
		baseURL:     baseURL,
		storageRoot: storageRoot,
		password:    cfg.Password,
		transport:   transport,
	}, nil
}

// FetchStudentPhoto 保存指定学校账号的 JPEG 照片，返回相对于上传根目录的路径。
func (c *Client) FetchStudentPhoto(ctx context.Context, username, captchaID, captchaAnswer string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("请提供学校用户名")
	}
	captchaID = strings.TrimSpace(captchaID)
	captchaAnswer = strings.TrimSpace(captchaAnswer)
	if captchaID == "" || captchaAnswer == "" {
		return "", errors.New("请提供验证码 ID 和答案")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{
		Jar:       jar,
		Transport: c.transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
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
