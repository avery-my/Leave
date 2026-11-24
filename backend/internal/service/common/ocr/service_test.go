package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRecognizeBase64DeadlineWhileWaitingForEngine(t *testing.T) {
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	input := base64.StdEncoding.EncodeToString(picture.Bytes())

	service := NewService(zap.NewNop())
	service.slot <- struct{}{}
	defer func() { <-service.slot }()

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := service.RecognizeBase64(ctx, input)
		result <- err
	}()

	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OCR request did not return while the engine was busy")
	}
}
