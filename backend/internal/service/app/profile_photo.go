package app

import (
	"context"
	"errors"
	"time"

	"backend/internal/utils/file"
)

func (p *ProfileService) fillMissingPhoto(ctx context.Context, studentID string) error {
	if p.schoolClient == nil || ctx.Err() != nil {
		return nil
	}
	profile, err := p.dao.GetProfileByStuID(studentID)
	if err != nil {
		return err
	}
	if profile == nil || profile.PhotoPath != "" {
		return nil
	}

	const maxAttempts = 5
	const attemptTimeout = 10 * time.Second
	var key string
	for range maxAttempts {
		if ctx.Err() != nil {
			return nil
		}
		attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
		key, err = p.schoolClient.FetchStudentPhoto(attemptCtx, studentID)
		cancel()
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return file.Delete(p.storage.RootDir, key)
	}

	updated, err := p.dao.UpdateProfilePhotoIfEmpty(ctx, profile.ID, key)
	if err != nil {
		// 连接中断时写入可能已经提交，确认路径未入库后再删除照片。
		if ctx.Err() != nil {
			return err
		}
		current, readErr := p.dao.GetProfileByStuID(studentID)
		if readErr != nil || (current != nil && current.PhotoPath == key) {
			return errors.Join(err, readErr)
		}
		return errors.Join(err, file.Delete(p.storage.RootDir, key))
	}
	if !updated {
		return file.Delete(p.storage.RootDir, key)
	}
	return nil
}
