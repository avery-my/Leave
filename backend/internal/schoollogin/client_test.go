package schoollogin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"backend/internal/config"
)

type testCaptchaRecognizer struct {
	answer string
	err    error
}

func (r testCaptchaRecognizer) RecognizeBase64(_ context.Context, input string) (string, error) {
	if input != "dGVzdA==" {
		return "", fmt.Errorf("unexpected captcha input: %q", input)
	}
	return r.answer, r.err
}

func TestFetchStudentPhoto(t *testing.T) {
	const photo = "\xff\xd8\xff\xd9"

	const script = `function _E() { return "\x30\x33"; }
function _M() { return "010000000001"; }
function _DK() { return "\x70\x3a"; }`
	key, err := parseLoginKey(script)
	if err != nil {
		t.Fatal(err)
	}

	ocrFailure := errors.New("test OCR failure")
	for _, tt := range []struct {
		name                  string
		status                int
		blockStorage          bool
		wantError             string
		wantSentinel          error
		loginError            string
		loginStatus           int
		ocr                   captchaRecognizer
		truncateFirstDownload bool
	}{
		{name: "saved", status: http.StatusOK},
		{name: "truncated download can be retried", status: http.StatusOK, truncateFirstDownload: true},
		{name: "upstream failure", status: http.StatusBadGateway, wantSentinel: ErrUpstreamUnavailable},
		{name: "photo missing", status: http.StatusNotFound, wantError: "HTTP 404"},
		{name: "captcha expired", loginError: "验证码已过期", wantSentinel: ErrCaptchaRejected},
		{name: "invalid credentials", loginError: "账号或密码错误", wantSentinel: errInvalidCredentials},
		{name: "login unavailable", loginStatus: http.StatusServiceUnavailable, wantSentinel: ErrUpstreamUnavailable},
		{name: "unknown login error", loginError: "test-token", wantError: "业务码 1"},
		{name: "storage failure", status: http.StatusOK, blockStorage: true, wantError: "保存学校照片"},
		{name: "OCR failure", ocr: testCaptchaRecognizer{err: ocrFailure}, wantError: "识别学校验证码"},
		{name: "empty OCR answer", ocr: testCaptchaRecognizer{answer: " "}, wantError: "学校验证码识别结果为空"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			usernames := make(chan string, 2)
			var downloadStarted atomic.Bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/captcha":
					if _, err := r.Cookie("session"); err == nil {
						t.Error("unexpected cookie from an earlier call")
					}
					http.SetCookie(w, &http.Cookie{Name: "session", Value: "test-session", Path: "/"})
					fmt.Fprint(w, `{"code":0,"message":"操作成功","data":{"cid":"captcha:test-id","base64":"dGVzdA=="}}`)
				case "/api/sec_js":
					if tt.ocr != nil {
						t.Error("login started without a valid captcha answer")
					}
					fmt.Fprint(w, script)
				case "/api/login":
					if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "test-session" {
						t.Error("login request missing session cookie")
					}
					var request loginRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Errorf("invalid login request: %v", err)
					}
					if request.CID != key.encrypt("captcha:test-id") || request.Code != key.encrypt("answer") {
						t.Error("login request did not use the fetched captcha and OCR answer")
					}
					usernames <- request.Username
					if tt.loginStatus != 0 {
						w.WriteHeader(tt.loginStatus)
						return
					}
					if tt.loginError != "" {
						fmt.Fprintf(w, `{"code":1,"message":%q}`, tt.loginError)
						return
					}
					fmt.Fprint(w, `{"code":0,"data":{"access_token":"test-token"}}`)
				case "/api/auth/user":
					if r.Header.Get("Authorization") != "test-token" {
						t.Error("user request missing token")
					}
					fmt.Fprint(w, `{"code":0,"data":{"userInfo":{"photo":"photo-id"}}}`)
				case "/api/file/inline/photo-id":
					if r.URL.Query().Get("access_token") != "test-token" {
						t.Error("photo request missing token")
					}
					w.Header().Set("Content-Type", "image/jpeg")
					if tt.truncateFirstDownload && !downloadStarted.Swap(true) {
						w.Header().Set("Content-Length", fmt.Sprint(len(photo)))
						fmt.Fprint(w, photo[:len(photo)-1])
						return
					}
					w.WriteHeader(tt.status)
					fmt.Fprint(w, photo)
				default:
					t.Errorf("unexpected request path: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			root := t.TempDir()
			photoDir := filepath.Join(root, "photo")
			if tt.blockStorage {
				if err := os.WriteFile(photoDir, []byte("existing file"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			recognizer := tt.ocr
			if recognizer == nil {
				recognizer = testCaptchaRecognizer{answer: "answer"}
			}
			client := NewClient(config.SchoolConfig{
				URL:      server.URL,
				Password: "test-password",
			}, root, recognizer)
			client.transport = server.Client().Transport

			if got, err := client.FetchStudentPhoto(t.Context(), " "); got != "" || err == nil || !strings.Contains(err.Error(), "用户名") {
				t.Fatalf("expected missing username error, got %q, %v", got, err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if got, err := client.FetchStudentPhoto(ctx, "student"); got != "" || !errors.Is(err, context.Canceled) {
				t.Fatalf("expected context cancellation, got %q, %v", got, err)
			}

			if tt.ocr != nil {
				got, err := client.FetchStudentPhoto(t.Context(), "student")
				if got != "" || err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected %q and no photo path, got %q, %v", tt.wantError, got, err)
				}
				if tt.name == "OCR failure" && !errors.Is(err, ocrFailure) {
					t.Fatalf("expected wrapped OCR error, got %v", err)
				}
				return
			}

			var previous string
			students := []string{"student-one", "student-two"}
			if tt.truncateFirstDownload {
				students[1] = students[0]
			}
			for attempt, username := range students {
				got, err := client.FetchStudentPhoto(t.Context(), username)
				select {
				case encrypted := <-usernames:
					if encrypted != key.encrypt(username) {
						t.Fatal("login did not use the supplied username")
					}
				default:
					t.Fatalf("login did not run: %v", err)
				}

				if tt.truncateFirstDownload && attempt == 0 {
					if got != "" || err != ErrUpstreamUnavailable {
						t.Fatalf("expected a retryable error and no photo path for a truncated download, got %q, %v", got, err)
					}
					if _, err := os.Stat(photoDir); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("truncated download should leave no photo directory, got %v", err)
					}
					continue
				}
				if tt.wantSentinel != nil {
					if got != "" || err != tt.wantSentinel {
						t.Fatalf("expected error %v and no photo path, got %q, %v", tt.wantSentinel, got, err)
					}
					continue
				}
				if tt.wantError != "" {
					if got != "" || err == nil || !strings.Contains(err.Error(), tt.wantError) || strings.Contains(err.Error(), "test-token") {
						t.Fatalf("expected %q with no photo path or token, got %q, %v", tt.wantError, got, err)
					}
					continue
				}
				if err != nil || filepath.ToSlash(filepath.Dir(got)) != "photo" || filepath.Ext(got) != ".jpg" || got == previous {
					t.Fatalf("expected a unique relative JPEG path, got %q, %v", got, err)
				}
				storedPhoto, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(got)))
				if err != nil || string(storedPhoto) != photo {
					t.Fatalf("expected saved JPEG bytes, got %q, %v", storedPhoto, err)
				}
				previous = got
			}
			if tt.status != http.StatusOK {
				if _, err := os.Stat(photoDir); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("upstream failure should leave no photo directory, got %v", err)
				}
			}
		})
	}
}
