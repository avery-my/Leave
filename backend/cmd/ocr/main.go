package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/getcharzp/go-ocr/paddle"
)

// 将普通图片的 Base64 或完整 Data URI 填在这里。
const imageBase64 = ""

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if imageBase64 == "" {
		return fmt.Errorf("请先给 main.go 中的 imageBase64 填入图片")
	}

	modelPath := findAsset(filepath.Join("model", "det.onnx"))
	if modelPath == "" {
		return fmt.Errorf("找不到 OCR 模型文件 model/det.onnx")
	}

	runtimePath := os.Getenv("OCR_ONNX_RUNTIME")
	if runtimePath == "" {
		var library string
		switch runtime.GOOS + "/" + runtime.GOARCH {
		case "darwin/arm64":
			library = "onnxruntime_arm64.dylib"
		case "linux/amd64":
			library = "onnxruntime_amd64.so"
		default:
			return fmt.Errorf("请用 OCR_ONNX_RUNTIME 指定当前平台的动态库绝对路径")
		}
		runtimePath = findAsset(filepath.Join("lib", library))
		if runtimePath == "" {
			return fmt.Errorf("找不到 lib/%s，请将动态库放入 lib 目录，或设置 OCR_ONNX_RUNTIME", library)
		}
	}

	if !filepath.IsAbs(runtimePath) {
		return fmt.Errorf("ONNX Runtime 动态库需要绝对路径")
	}

	img, err := decodeImage(imageBase64)
	if err != nil {
		return err
	}

	modelDir := filepath.Dir(modelPath)
	engine, err := paddle.NewEngine(paddle.Config{
		OnnxRuntimeLibPath: runtimePath,
		DetModelPath:       modelPath,
		RecModelPath:       filepath.Join(modelDir, "rec.onnx"),
		DictPath:           filepath.Join(modelDir, "ppocr_keys_v1.txt"),
		NumThreads:         1,
		DetMaxSideLen:      640,
		// 保留文字边缘和字间距，避免裁剪过紧。
		DetOutsideExpandPix: 24,
	})
	if err != nil {
		return fmt.Errorf("加载 OCR 模型或运行库: %w", err)
	}
	defer engine.Destroy()

	result, err := engine.RunOCR(img)
	if err != nil {
		return fmt.Errorf("OCR 推理失败: %w", err)
	}
	lines := make([]string, 0, len(result))
	for _, region := range result {
		if line := strings.TrimSpace(region.Text); line != "" {
			lines = append(lines, line)
		}
	}
	_, err = fmt.Fprintln(os.Stdout, strings.Join(lines, "\n"))
	return err
}

func findAsset(name string) string {
	executable, _ := os.Executable()
	_, source, _, _ := runtime.Caller(0)
	// GoLand 把可执行文件放在缓存目录，开发时回退到源码目录。
	for _, file := range []string{executable, source} {
		if !filepath.IsAbs(file) {
			continue
		}
		path := filepath.Join(filepath.Dir(file), name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}
