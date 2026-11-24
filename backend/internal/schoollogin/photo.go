package schoollogin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func fetchPhotoID(ctx context.Context, client *http.Client, baseURL, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/auth/user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", baseURL+"/checkin/workbench")
	req.Header.Set("Authorization", token)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求用户资料: %w", err)
	}
	defer resp.Body.Close()
	if err := schoolHTTPError(resp.StatusCode, "请求用户资料"); err != nil {
		return "", err
	}

	var result struct {
		Code *int `json:"code"`
		Data struct {
			UserInfo struct {
				Photo string `json:"photo"`
			} `json:"userInfo"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return "", fmt.Errorf("解析用户资料响应: %w", err)
	}
	if result.Code == nil {
		return "", errors.New("用户资料响应缺少业务状态码")
	}
	if *result.Code != 0 {
		return "", fmt.Errorf("获取用户资料失败 (业务码 %d)", *result.Code)
	}
	photoID, err := parsePhotoID(result.Data.UserInfo.Photo)
	if err != nil {
		return "", fmt.Errorf("解析 data.userInfo.photo: %w", err)
	}
	return photoID, nil
}

func parsePhotoID(photo string) (string, error) {
	photoID := strings.TrimSpace(photo)
	if strings.HasPrefix(photoID, "[") {
		var files []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(photoID), &files); err != nil {
			return "", errors.New("照片附件数组格式异常")
		}
		if len(files) == 0 {
			return "", errors.New("照片附件数组为空")
		}
		photoID = strings.TrimSpace(files[0].ID)
	}
	if photoID == "" || photoID == "." || photoID == ".." || strings.ContainsAny(photoID, "/\\") {
		return "", errors.New("照片标识无效")
	}
	return photoID, nil
}

func downloadPhoto(ctx context.Context, client *http.Client, baseURL, token, photoID string) ([]byte, error) {
	query := url.Values{"access_token": {token}}
	photoURL := baseURL + "/api/file/inline/" + url.PathEscape(photoID) + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, photoURL, nil)
	if err != nil {
		return nil, errors.New("构建照片请求失败")
	}
	req.Header.Set("Referer", baseURL+"/checkin/workbench")
	req.Header.Set("Accept", "image/*")
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// URL 错误包含完整请求地址，查询参数里有令牌。
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, context.DeadlineExceeded
		}
		return nil, errPhotoRequest
	}
	defer resp.Body.Close()
	if err := schoolHTTPError(resp.StatusCode, "下载照片"); err != nil {
		return nil, err
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "image/jpeg" {
		return nil, errors.New("照片响应格式异常，预期 JPEG")
	}

	const maxPhotoBytes = 20 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPhotoBytes+1))
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrUpstreamUnavailable
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, context.DeadlineExceeded
		}
		return nil, errPhotoRead
	}
	if len(data) == 0 || len(data) > maxPhotoBytes {
		return nil, errors.New("照片大小超出允许范围")
	}
	return data, nil
}
