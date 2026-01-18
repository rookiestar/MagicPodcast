package utils

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"

	"github.com/skip2/go-qrcode"
)

// GenerateQRCodeForEpisode 为episode生成小宇宙二维码（base64编码）
// xyzID: episode的小宇宙ID
// size: 二维码尺寸（像素），建议128或256
func GenerateQRCodeForEpisode(xyzID string, size int) (string, error) {
	// 构造小宇宙URL
	url := fmt.Sprintf("https://www.xiaoyuzhoufm.com/episode/%s", xyzID)

	// 生成二维码（使用Medium纠错级别）
	qr, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return "", fmt.Errorf("生成二维码失败: %w", err)
	}

	// 转为Image（设置尺寸）
	qrImage := qr.Image(size)

	// 转为PNG图片
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, qrImage); err != nil {
		return "", fmt.Errorf("编码PNG失败: %w", err)
	}

	// 转为base64
	base64Str := base64.StdEncoding.EncodeToString(buf.Bytes())
	return fmt.Sprintf("data:image/png;base64,%s", base64Str), nil
}
