package app

import (
	"context"
	"errors"
	"time"

	"backend/internal/utils/file"
)

type profilePhotoImport struct {
	done chan struct{}
	err  error
}

func (p *ProfileService) fillMissingPhoto(ctx context.Context, studentID string) error {
	if p.schoolClient == nil || ctx.Err() != nil {
		return nil
	}

	job := &profilePhotoImport{done: make(chan struct{})}
	if active, loaded := p.photoImports.LoadOrStore(studentID, job); loaded {
		current := active.(*profilePhotoImport)
		select {
		case <-current.done:
			return current.err
		case <-ctx.Done():
			return nil
		}
	}
	defer func() {
		p.photoImports.Delete(studentID)
		close(job.done)
	}()
	job.err = p.importMissingPhoto(ctx, studentID)
	return job.err
}

func (p *ProfileService) importMissingPhoto(ctx context.Context, studentID string) error {
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
	return p.persistImportedPhoto(ctx, profile.ID, key)
}

func (p *ProfileService) persistImportedPhoto(ctx context.Context, profileID uint, key string) error {
	// 此时照片已落盘，即使客户端断开，也要完成写入或清理。
	const databaseTimeout = 3 * time.Second
	background := context.WithoutCancel(ctx)
	writeCtx, cancelWrite := context.WithTimeout(background, databaseTimeout)
	updated, err := p.dao.UpdateProfilePhotoIfEmpty(writeCtx, profileID, key)
	cancelWrite()
	if err != nil {
		// 连接中断时写入可能已经提交，确认路径未入库后再删除照片。
		checkCtx, cancelCheck := context.WithTimeout(background, databaseTimeout)
		referenced, readErr := p.dao.ProfileReferencesPhoto(checkCtx, profileID, key)
		cancelCheck()
		if readErr != nil || referenced {
			return errors.Join(err, readErr)
		}
		return errors.Join(err, file.Delete(p.storage.RootDir, key))
	}
	if !updated {
		return file.Delete(p.storage.RootDir, key)
	}
	return nil
}
