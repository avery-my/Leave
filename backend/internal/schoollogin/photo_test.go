package schoollogin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type photoTestTransport func(*http.Request) (*http.Response, error)

func (f photoTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDownloadPhotoTransportError(t *testing.T) {
	const token = "test-token"
	for _, tt := range []struct {
		name string
		err  error
		want error
	}{
		{name: "timeout", err: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "unknown error", err: errors.New("unexpected transport error"), want: errPhotoRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{
				Transport: photoTestTransport(func(_ *http.Request) (*http.Response, error) {
					return nil, tt.err
				}),
			}
			_, err := downloadPhoto(t.Context(), client, "https://school.example.test", token, "photo-id")
			if err != tt.want || strings.Contains(err.Error(), token) {
				t.Fatalf("expected a safe download error %v, got %v", tt.want, err)
			}
		})
	}
}

func TestParsePhotoID(t *testing.T) {
	for _, test := range []struct {
		name  string
		photo string
		want  string
	}{
		{name: "attachment JSON", photo: `[{"contentType":"image/jpeg","id":"abc123","originalFilename":"student.jpg","size":28191}]`, want: "abc123"},
		{name: "bare ID", photo: "abc123", want: "abc123"},
		{name: "empty array", photo: "[]"},
		{name: "malformed array", photo: "["},
		{name: "missing ID", photo: `[{"contentType":"image/jpeg"}]`},
		{name: "invalid ID", photo: `[{"id":"folder/image"}]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parsePhotoID(test.photo)
			if got != test.want || (err != nil) != (test.want == "") {
				t.Fatalf("parsePhotoID = (%q, %v), want ID %q", got, err, test.want)
			}
		})
	}
}
