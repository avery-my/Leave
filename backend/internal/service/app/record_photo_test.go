package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"backend/internal/config"
	"backend/internal/dao/mysql"
	"backend/internal/models"
	"backend/internal/schoollogin"
	"backend/internal/utils/file"

	mysqldriver "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type testStudentPhotoClient func(context.Context, string) (string, error)

func (f testStudentPhotoClient) FetchStudentPhoto(ctx context.Context, username string) (string, error) {
	return f(ctx, username)
}

type photoImportWaitContext struct {
	context.Context
	waiting chan struct{}
}

func (c photoImportWaitContext) Done() <-chan struct{} {
	close(c.waiting)
	return c.Context.Done()
}

// DryRun 仍会开启显式事务，替换事务入口以免连接测试占位地址。
type photoTestConnPool struct {
	gorm.ConnPool
}

func (p photoTestConnPool) BeginTx(context.Context, *sql.TxOptions) (gorm.ConnPool, error) {
	return p, nil
}

func (photoTestConnPool) Commit() error   { return nil }
func (photoTestConnPool) Rollback() error { return nil }

func TestGetRecordPhotoImport(t *testing.T) {
	databaseErr := &mysqldriver.MySQLError{Number: 1205, Message: "database write result unknown"}
	for _, tt := range []struct {
		name            string
		succeedOn       int
		cancelOn        int
		fetchErr        error
		wantCalls       int
		photoPath       string
		missingProfile  bool
		schoolDisabled  bool
		requestedBy     string
		missingRecord   bool
		writeSkipped    bool
		writeErr        error
		cancelOnWrite   bool
		photoReferenced bool
		checkErr        error
		blockCleanup    bool
		concurrent      bool
		wantFile        bool
		wantWarnings    []string
	}{
		{name: "first success", succeedOn: 1, wantCalls: 1, wantFile: true},
		{name: "fifth attempt succeeds", succeedOn: 5, wantCalls: 5, wantFile: true},
		{name: "network timeout retries", fetchErr: context.DeadlineExceeded, succeedOn: 2, wantCalls: 2, wantFile: true},
		{name: "wrapped HTTP timeout retries", fetchErr: fmt.Errorf("fetch captcha: %w", context.DeadlineExceeded), succeedOn: 2, wantCalls: 2, wantFile: true},
		{name: "temporary upstream error retries", fetchErr: schoollogin.ErrUpstreamUnavailable, succeedOn: 2, wantCalls: 2, wantFile: true},
		{name: "success stops further attempts", succeedOn: 3, wantCalls: 3, wantFile: true},
		{name: "five temporary errors", wantCalls: 5, wantWarnings: []string{"补录学生照片失败"}},
		{name: "permanent error stops retries", fetchErr: errors.New("OCR model missing"), wantCalls: 1, wantWarnings: []string{"补录学生照片失败"}},
		{name: "concurrent requests share failed import", concurrent: true, wantCalls: 5, wantWarnings: []string{"补录学生照片失败", "补录学生照片失败"}},
		{name: "concurrent requests share successful import", concurrent: true, succeedOn: 1, wantCalls: 1, wantFile: true},
		{name: "request canceled on second attempt", cancelOn: 2, wantCalls: 2},
		{name: "request canceled on final attempt", cancelOn: 5, wantCalls: 5},
		{name: "photo already exists", photoPath: "photo/existing.jpg"},
		{name: "missing profile", missingProfile: true},
		{name: "school disabled", schoolDisabled: true},
		{name: "missing record", missingRecord: true},
		{name: "record belongs to another student", requestedBy: "student-b"},
		{name: "concurrent update won", succeedOn: 1, wantCalls: 1, writeSkipped: true},
		{name: "write failed after affecting a row", succeedOn: 1, wantCalls: 1, writeErr: databaseErr, wantWarnings: []string{"补录学生照片失败"}},
		{name: "write committed before error", succeedOn: 1, wantCalls: 1, writeErr: databaseErr, photoReferenced: true, wantFile: true},
		{name: "client disconnect during successful write", succeedOn: 1, wantCalls: 1, cancelOnWrite: true, wantFile: true},
		{name: "client disconnect during failed write", succeedOn: 1, wantCalls: 1, cancelOnWrite: true, writeErr: databaseErr, wantWarnings: []string{"补录学生照片失败"}},
		{name: "client disconnect during ambiguous commit", succeedOn: 1, wantCalls: 1, cancelOnWrite: true, writeErr: databaseErr, photoReferenced: true, wantFile: true},
		{name: "cannot confirm whether file is referenced", succeedOn: 1, wantCalls: 1, writeErr: databaseErr, checkErr: errors.New("check timed out"), wantFile: true, wantWarnings: []string{"核对补录照片路径失败，保留文件", "补录学生照片失败"}},
		{name: "failed write and failed cleanup are logged separately", succeedOn: 1, wantCalls: 1, writeErr: databaseErr, blockCleanup: true, wantFile: true, wantWarnings: []string{"清理补录照片失败", "补录学生照片失败"}},
		{name: "failed cleanup after losing concurrent update", succeedOn: 1, wantCalls: 1, writeSkipped: true, blockCleanup: true, wantFile: true, wantWarnings: []string{"清理补录照片失败"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const studentID = "student-a"
			const recordID = 17
			const profileID = 21
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			storage := config.StorageConfig{RootDir: t.TempDir()}
			var key string
			var profileReads, photoCalls, writes int
			var followerDone chan error
			var service *RecordService
			db, err := gorm.Open(gormmysql.New(gormmysql.Config{
				DSN:                       "tcp(127.0.0.1:1)/isolated_record_photo_test",
				SkipInitializeWithVersion: true,
			}), &gorm.Config{
				DryRun:                 true,
				DisableAutomaticPing:   true,
				SkipDefaultTransaction: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			connection, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			db.Statement.ConnPool = photoTestConnPool{ConnPool: connection}
			if err := db.Callback().Query().After("gorm:query").Register("test:record_photo_query", func(tx *gorm.DB) {
				switch dest := tx.Statement.Dest.(type) {
				case *models.Record:
					if !strings.Contains(tx.Statement.SQL.String(), "id = ? AND student_id = ?") {
						t.Fatal("record query must check ownership")
					}
					if tt.missingRecord || tx.Statement.Vars[1] != studentID {
						tx.AddError(gorm.ErrRecordNotFound)
						return
					}
					*dest = models.Record{ID: recordID, StudentID: studentID}
				case *models.Profile:
					profileReads++
					if writes > 0 {
						if tx.Statement.Vars[0] != uint(profileID) || tx.Statement.Context.Err() != nil ||
							!strings.Contains(tx.Statement.SQL.String(), "FOR UPDATE") {
							t.Fatalf("expected an active check for profile %d, got %s %v", profileID, tx.Statement.SQL.String(), tx.Statement.Vars)
						}
						if tt.checkErr != nil {
							tx.AddError(tt.checkErr)
							return
						}
						if tt.photoReferenced {
							dest.PhotoPath = key
						}
						return
					}
					if tx.Statement.Vars[0] != studentID {
						t.Fatalf("profile lookup used %v", tx.Statement.Vars[0])
					}
					if tt.missingProfile {
						tx.AddError(gorm.ErrRecordNotFound)
						return
					}
					*dest = models.Profile{ID: profileID, StudentID: studentID, PhotoPath: tt.photoPath}
				}
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.Callback().Update().After("gorm:update").Register("test:record_photo_update", func(tx *gorm.DB) {
				writes++
				if tt.cancelOnWrite {
					cancel()
					if tx.Statement.Context.Err() != nil {
						t.Fatal("client disconnect canceled the photo update")
					}
				}
				if !strings.Contains(tx.Statement.SQL.String(), "id = ? AND (photo_path IS NULL OR photo_path = '')") ||
					tx.Statement.Vars[0] != key || tx.Statement.Vars[len(tx.Statement.Vars)-1] != uint(profileID) {
					t.Fatalf("unexpected photo update %s %v", tx.Statement.SQL.String(), tx.Statement.Vars)
				}
				if !tt.writeSkipped {
					tx.RowsAffected = 1
				}
				if tt.writeErr != nil {
					tx.AddError(tt.writeErr)
				}
			}); err != nil {
				t.Fatal(err)
			}
			fetchPhoto := func(attemptCtx context.Context, username string) (string, error) {
				photoCalls++
				if tt.concurrent && photoCalls == 1 {
					waiting := make(chan struct{})
					followerDone = make(chan error, 1)
					go func() {
						followerCtx := photoImportWaitContext{Context: ctx, waiting: waiting}
						_, err := service.GetRecord(followerCtx, studentID, recordID)
						followerDone <- err
					}()
					select {
					case <-waiting:
					case <-attemptCtx.Done():
						t.Fatal("concurrent request did not wait for the active import")
					}
				}
				if username != studentID {
					t.Fatalf("unexpected School username %q", username)
				}
				if deadline, ok := attemptCtx.Deadline(); !ok || time.Until(deadline) > 10*time.Second {
					t.Fatal("School call must have a deadline of at most ten seconds")
				}
				if photoCalls == tt.cancelOn {
					cancel()
					return "", attemptCtx.Err()
				}
				if photoCalls != tt.succeedOn {
					if tt.fetchErr != nil {
						return "", tt.fetchErr
					}
					return "", schoollogin.ErrCaptchaRejected
				}
				keys, err := file.Save(storage.RootDir, "photo", file.Input{
					Extension: ".jpg",
					Content:   strings.NewReader("test photo"),
				})
				if err != nil {
					t.Fatal(err)
				}
				key = keys[0]
				if tt.blockCleanup {
					// 用非空目录模拟无法删除的路径，避免测试依赖运行用户权限。
					path := filepath.Join(storage.RootDir, filepath.FromSlash(key))
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(path, "block"), nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
				return key, nil
			}
			var schoolClient StudentPhotoClient
			if !tt.schoolDisabled {
				schoolClient = testStudentPhotoClient(fetchPhoto)
			}
			core, logs := observer.New(zap.WarnLevel)
			logger := zap.New(core)
			dao := mysql.NewDAO(db, logger)
			service = NewRecordService(dao, NewProfileService(dao, storage, schoolClient, logger), logger)
			requester := studentID
			if tt.requestedBy != "" {
				requester = tt.requestedBy
			}
			response, err := service.GetRecord(ctx, requester, recordID)
			if followerDone != nil {
				select {
				case err := <-followerDone:
					if err != nil || profileReads != 1 {
						t.Fatalf("concurrent request did not share the import: %v, profile reads %d", err, profileReads)
					}
				case <-time.After(time.Second):
					t.Fatal("concurrent request did not return after the import")
				}
			}
			switch {
			case tt.missingRecord || requester != studentID:
				if !errors.Is(err, mysql.ErrorRecordNotExist) || response != nil || profileReads != 0 {
					t.Fatalf("unexpected result for unavailable record: %+v, %v, reads %d", response, err, profileReads)
				}
			case err != nil || response == nil || response.ID != recordID:
				t.Fatalf("unexpected detail response: %+v, %v", response, err)
			}
			if photoCalls != tt.wantCalls {
				t.Fatalf("School calls = %d, want %d", photoCalls, tt.wantCalls)
			}
			wantWrites := 0
			if tt.succeedOn != 0 {
				wantWrites = 1
			}
			if writes != wantWrites {
				t.Fatalf("writes = %d, want %d", writes, wantWrites)
			}
			warnings := logs.All()
			if len(warnings) != len(tt.wantWarnings) {
				t.Fatalf("warnings = %v, want %v", warnings, tt.wantWarnings)
			}
			for i, warning := range warnings {
				if warning.Message != tt.wantWarnings[i] {
					t.Fatalf("warning = %q, want %q", warning.Message, tt.wantWarnings[i])
				}
				if warning.Message != "补录学生照片失败" && warning.ContextMap()["file_key"] != key {
					t.Fatalf("warning should identify the retained file: %v", warning.ContextMap())
				}
			}
			if key != "" {
				_, err := os.Stat(filepath.Join(storage.RootDir, filepath.FromSlash(key)))
				if (tt.wantFile && err != nil) || (!tt.wantFile && !errors.Is(err, os.ErrNotExist)) {
					t.Fatalf("photo file should exist: %t, stat error: %v", tt.wantFile, err)
				}
			}
		})
	}
}
