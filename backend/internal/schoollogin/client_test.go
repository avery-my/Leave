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
	"testing"

	"backend/internal/config"
)

func TestNewClientValidatesConfig(t *testing.T) {
	for _, tt := range []struct {
		name     string
		url      string
		root     string
		password string
		want     string
	}{
		{name: "missing URL", want: "SCHOOL_URL"},
		{name: "HTTP", url: "http://school.example.test", want: "HTTPS"},
		{name: "URL with path", url: "https://school.example.test/login", want: "HTTPS"},
		{name: "missing password", url: "https://school.example.test", want: "SCHOOL_PASSWORD"},
		{name: "missing storage root", url: "https://school.example.test", password: "test-password", want: "storage.root_dir"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(config.SchoolConfig{
				URL:      tt.url,
				Password: tt.password,
			}, tt.root)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewClient() error = %v, want %s", err, tt.want)
			}
		})
	}
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

	for _, tt := range []struct {
		name         string
		status       int
		blockStorage bool
		wantError    string
	}{
		{name: "saved", status: http.StatusOK},
		{name: "upstream failure", status: http.StatusBadGateway, wantError: "HTTP 502"},
		{name: "storage failure", status: http.StatusOK, blockStorage: true, wantError: "保存学校照片"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			usernames := make(chan string, 2)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/sec_js":
					if _, err := r.Cookie("session"); err == nil {
						t.Error("unexpected cookie from an earlier call")
					}
					http.SetCookie(w, &http.Cookie{Name: "session", Value: "test-session", Path: "/"})
					fmt.Fprint(w, script)
				case "/api/login":
					if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "test-session" {
						t.Error("login request missing session cookie")
					}
					var request loginRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Errorf("invalid login request: %v", err)
					}
					usernames <- request.Username
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
			client, err := NewClient(config.SchoolConfig{
				URL:      server.URL,
				Password: "test-password",
			}, root)
			if err != nil {
				t.Fatal(err)
			}
			client.transport = server.Client().Transport

			if got, err := client.FetchStudentPhoto(t.Context(), " ", "captcha-id", "answer"); got != "" || err == nil || !strings.Contains(err.Error(), "用户名") {
				t.Fatalf("expected missing username error, got %q, %v", got, err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if got, err := client.FetchStudentPhoto(ctx, "student", "captcha-id", "answer"); got != "" || !errors.Is(err, context.Canceled) {
				t.Fatalf("expected context cancellation, got %q, %v", got, err)
			}

			var previous string
			for _, username := range []string{"student-one", "student-two"} {
				got, err := client.FetchStudentPhoto(t.Context(), username, "captcha-id", "answer")
				select {
				case encrypted := <-usernames:
					if encrypted != key.encrypt(username) {
						t.Fatal("login did not use the supplied username")
					}
				default:
					t.Fatalf("login did not run: %v", err)
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
