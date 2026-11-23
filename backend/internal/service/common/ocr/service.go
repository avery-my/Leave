package ocr

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	logutil "backend/internal/logger"

	"github.com/getcharzp/go-ocr/paddle"
	"go.uber.org/zap"
)

// Service 管理一个按需加载的 OCR 引擎。
type Service struct {
	logger *zap.Logger
	engine *paddle.Engine
	mu     sync.Mutex
}

func NewService(logger *zap.Logger) *Service {
	return &Service{
		logger: logger,
	}
}

// Close 释放已加载的引擎。再次识别时会重新加载。
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine != nil {
		s.engine.Destroy()
		s.engine = nil
	}
}

// RecognizeBase64 识别 PNG 或 JPEG 的 Base64 或 Data URI，直接拼接各段识别文本。
func (s *Service) RecognizeBase64(input string) (string, error) {
	img, err := decodeImage(input)
	if err != nil {
		return "", err
	}

	// 首次使用时加载模型，并串行访问同一实例的引擎。
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine == nil {
		s.engine, err = loadOCREngine()
		if err != nil {
			s.logger.Error("OCR 引擎初始化失败",
				zap.String("operation", "ocr.initialize"),
				logutil.Error(err),
			)
			return "", err
		}
		s.logger.Info("OCR 引擎初始化成功", zap.String("operation", "ocr.initialize"))
	}

	result, err := s.engine.RunOCR(img)
	if err != nil {
		s.logger.Error("OCR 推理失败",
			zap.String("operation", "ocr.recognize"),
			logutil.Error(err),
		)
		return "", fmt.Errorf("OCR 推理失败: %w", err)
	}

	var text strings.Builder
	for _, region := range result {
		text.WriteString(strings.TrimSpace(region.Text))
	}
	return text.String(), nil
}

func loadOCREngine() (*paddle.Engine, error) {
	modelPath := findOCRAsset(filepath.Join("model", "det.onnx"))
	if modelPath == "" {
		return nil, fmt.Errorf("找不到 OCR 模型文件 model/det.onnx")
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
			return nil, fmt.Errorf("请用 OCR_ONNX_RUNTIME 指定当前平台的动态库绝对路径")
		}
		runtimePath = findOCRAsset(filepath.Join("lib", library))
		if runtimePath == "" {
			return nil, fmt.Errorf("找不到 lib/%s，请将动态库放入 lib 目录，或设置 OCR_ONNX_RUNTIME", library)
		}
	}

	if !filepath.IsAbs(runtimePath) {
		return nil, fmt.Errorf("ONNX Runtime 动态库需要绝对路径")
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
		return nil, fmt.Errorf("加载 OCR 模型或运行库: %w", err)
	}
	return engine, nil
}

func findOCRAsset(name string) string {
	// 本地支持从 backend 或仓库根目录启动。
	dirs := []string{
		"assets/ocr",
		"backend/assets/ocr",
	}
	if executable, err := os.Executable(); err == nil {
		// current 指向 backend/releases/vN，OCR 资源由各环境根目录共享。
		if resolved, err := filepath.EvalSymlinks(executable); err == nil {
			releasesDir := filepath.Dir(filepath.Dir(resolved))
			backendDir := filepath.Dir(releasesDir)
			if filepath.Base(releasesDir) == "releases" && filepath.Base(backendDir) == "backend" {
				dirs = append([]string{filepath.Join(filepath.Dir(backendDir), "assets/ocr")}, dirs...)
			}
		}
	}
	for _, dir := range dirs {
		path, err := filepath.Abs(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}
