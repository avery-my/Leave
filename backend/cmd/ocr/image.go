package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

const (
	maxImageBytes = 8 << 20
	maxPixels     = 8 << 20
	maxSide       = 4096
)

func decodeImage(input string) (image.Image, error) {
	input = strings.TrimSpace(input)
	var declared string
	if strings.HasPrefix(input, "data:") {
		header, payload, ok := strings.Cut(input, ",")
		if !ok {
			return nil, fmt.Errorf("无效的图片 Data URI：缺少逗号")
		}
		switch strings.ToLower(header) {
		case "data:image/png;base64":
			declared = "png"
		case "data:image/jpeg;base64":
			declared = "jpeg"
		default:
			return nil, fmt.Errorf("不支持的图片 Data URI 类型")
		}
		input = payload
	}

	if len(input) > base64.StdEncoding.EncodedLen(maxImageBytes) {
		return nil, fmt.Errorf("图片超出大小限制")
	}
	data, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return nil, fmt.Errorf("无效的 Base64: %w", err)
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("图片超出大小限制")
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("无效的 PNG 或 JPEG 图片: %w", err)
	}
	if (format != "png" && format != "jpeg") || (declared != "" && declared != format) {
		return nil, fmt.Errorf("图片内容与声明的格式不符")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("无效的图片尺寸")
	}
	if cfg.Width > maxSide || cfg.Height > maxSide || cfg.Width*cfg.Height > maxPixels {
		return nil, fmt.Errorf("图片超出大小限制")
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("无效的 PNG 或 JPEG 图片: %w", err)
	}
	// 透明 PNG 用白色背景合成，避免缩放时透明区域变黑。
	if opaque, ok := img.(interface{ Opaque() bool }); !ok || !opaque.Opaque() {
		background := image.NewRGBA(img.Bounds())
		draw.Draw(background, background.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		draw.Draw(background, background.Bounds(), img, img.Bounds().Min, draw.Over)
		img = background
	}
	return img, nil
}
