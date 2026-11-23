package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"
)

const (
	schoolURL      = "https://xgxtv5.imut.edu.cn"
	schoolUsername = ""
	schoolPassword = ""
	captchaID      = ""
	captchaAnswer  = ""
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
	CID      string `json:"cid"`
	Remember bool   `json:"remember"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("学校登录接口返回成功")
}

func run() error {
	username := strings.TrimSpace(schoolUsername)
	if username == "" || schoolPassword == "" {
		return errors.New("请在 main.go 填写 schoolUsername 和 schoolPassword")
	}
	if captchaID == "" || strings.TrimSpace(captchaAnswer) == "" {
		return errors.New("请在 main.go 填写 captchaID 和 captchaAnswer")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Jar:       jar,
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return login(client, schoolURL, username, schoolPassword, captchaID, strings.TrimSpace(captchaAnswer))
}

func login(client *http.Client, baseURL, username, password, cid, answer string) error {
	scriptURL := fmt.Sprintf("%s/api/sec_js?t=%d", baseURL, time.Now().UnixMilli())
	script, err := client.Get(scriptURL)
	if err != nil {
		return fmt.Errorf("获取安全脚本: %w", err)
	}
	defer script.Body.Close()
	if script.StatusCode != http.StatusOK {
		return fmt.Errorf("获取安全脚本失败 (HTTP %d)", script.StatusCode)
	}
	source, err := io.ReadAll(io.LimitReader(script.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读取安全脚本: %w", err)
	}
	key, err := parseLoginKey(string(source))
	if err != nil {
		return err
	}

	payload, err := json.Marshal(loginRequest{
		Username: key.encrypt(username),
		Password: key.encrypt(password),
		Code:     key.encrypt(answer),
		CID:      key.encrypt(cid),
		Remember: false,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/login", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("提交登录: %w", err)
	}
	defer resp.Body.Close()
	var result struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result)
	if resp.StatusCode != http.StatusOK || (decodeErr == nil && result.Code != nil && *result.Code != 0) {
		status := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if decodeErr == nil && result.Code != nil {
			status += fmt.Sprintf("，业务码 %d", *result.Code)
		}
		// 只显示可识别的错误原因。响应正文和其他字段可能包含会话信息。
		if decodeErr == nil {
			for _, reason := range []string{"参数解码错误", "验证码已过期", "验证码错误", "验证码不正确", "账号或密码错误", "用户名或密码错误"} {
				if strings.Contains(result.Message, reason) {
					return fmt.Errorf("登录失败 (%s) %s", status, reason)
				}
			}
		}
		return fmt.Errorf("登录失败 (%s)", status)
	}
	if decodeErr != nil {
		return errors.New("登录接口响应格式异常")
	}
	if result.Code == nil {
		return errors.New("登录接口响应缺少业务状态码")
	}
	return nil
}
