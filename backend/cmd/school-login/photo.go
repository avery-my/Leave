package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func fetchPhotoID(client *http.Client, baseURL, token string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/auth/user", nil)
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
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("请求用户资料失败 (HTTP %d)", resp.StatusCode)
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
		return "", errors.New("用户资料响应格式异常")
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

func downloadPhoto(client *http.Client, baseURL, token, photoID string) (string, error) {
	query := url.Values{"access_token": {token}}
	photoURL := baseURL + "/api/file/inline/" + url.PathEscape(photoID) + "?" + query.Encode()
	req, err := http.NewRequest(http.MethodGet, photoURL, nil)
	if err != nil {
		return "", errors.New("构建照片请求失败")
	}
	req.Header.Set("Referer", baseURL+"/checkin/workbench")
	req.Header.Set("Accept", "image/*")
	resp, err := client.Do(req)
	if err != nil {
		// URL 错误包含完整请求地址，查询参数里有令牌。
		return "", errors.New("请求照片失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载照片失败 (HTTP %d)", resp.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "image/jpeg" {
		return "", errors.New("照片响应格式异常，预期 JPEG")
	}

	file, err := os.CreateTemp(schoolLoginDir, "photo-*.jpg")
	if err != nil {
		return "", fmt.Errorf("创建照片文件: %w", err)
	}
	const maxPhotoBytes = 20 << 20
	size, copyErr := io.Copy(file, io.LimitReader(resp.Body, maxPhotoBytes+1))
	closeErr := file.Close()
	if size == 0 || size > maxPhotoBytes {
		copyErr = errors.Join(copyErr, errors.New("照片大小超出允许范围"))
	}
	if err := errors.Join(copyErr, closeErr); err != nil {
		return "", fmt.Errorf("保存照片: %w", errors.Join(err, os.Remove(file.Name())))
	}
	return file.Name(), nil
}
