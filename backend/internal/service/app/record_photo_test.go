package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"backend/internal/config"
	"backend/internal/dao/mysql"
	"backend/internal/models"
	"backend/internal/utils/file"

	"go.uber.org/zap"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type testStudentPhotoClient func(context.Context, string) (string, error)

func (f testStudentPhotoClient) FetchStudentPhoto(ctx context.Context, username string) (string, error) {
	return f(ctx, username)
}

func TestGetRecordPhotoImport(t *testing.T) {
	databaseErr := errors.New("database write result unknown")
	for _, tt := range []struct {
		name            string
		succeedOn       int
		cancelOn        int
		wantCalls       int
		photoPath       string
		missingProfile  bool
		schoolDisabled  bool
		requestedBy     string
		missingRecord   bool
		writeSkipped    bool
		writeErr        error
		photoReferenced bool
		wantFile        bool
	}{
		{name: "first success", succeedOn: 1, wantCalls: 1, wantFile: true},
		{name: "fifth attempt succeeds", succeedOn: 5, wantCalls: 5, wantFile: true},
		{name: "success stops further attempts", succeedOn: 3, wantCalls: 3, wantFile: true},
		{name: "five errors", wantCalls: 5},
		{name: "request canceled", cancelOn: 2, wantCalls: 2},
		{name: "photo already exists", photoPath: "photo/existing.jpg"},
		{name: "missing profile", missingProfile: true},
		{name: "school disabled", schoolDisabled: true},
		{name: "missing record", missingRecord: true},
		{name: "record belongs to another student", requestedBy: "student-b"},
		{name: "concurrent update won", succeedOn: 1, wantCalls: 1, writeSkipped: true},
		{name: "write failed", succeedOn: 1, wantCalls: 1, writeErr: databaseErr},
		{name: "write committed before error", succeedOn: 1, wantCalls: 1, writeErr: databaseErr, photoReferenced: true, wantFile: true},
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
					if tx.Statement.Vars[0] != studentID {
						t.Fatalf("profile lookup used %v", tx.Statement.Vars[0])
					}
					if tt.missingProfile {
						tx.AddError(gorm.ErrRecordNotFound)
						return
					}
					*dest = models.Profile{ID: profileID, StudentID: studentID, PhotoPath: tt.photoPath}
					if tt.photoReferenced && writes > 0 {
						dest.PhotoPath = key
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.Callback().Update().After("gorm:update").Register("test:record_photo_update", func(tx *gorm.DB) {
				writes++
				if !strings.Contains(tx.Statement.SQL.String(), "id = ? AND (photo_path IS NULL OR photo_path = '')") ||
					tx.Statement.Vars[0] != key || tx.Statement.Vars[len(tx.Statement.Vars)-1] != uint(profileID) {
					t.Fatalf("unexpected photo update %s %v", tx.Statement.SQL.String(), tx.Statement.Vars)
				}
				if tt.writeErr != nil {
					tx.AddError(tt.writeErr)
				} else if !tt.writeSkipped {
					tx.RowsAffected = 1
				}
			}); err != nil {
				t.Fatal(err)
			}
			fetchPhoto := func(attemptCtx context.Context, username string) (string, error) {
				photoCalls++
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
					return "", errors.New("School rejected captcha")
				}
				keys, err := file.Save(storage.RootDir, "photo", file.Input{
					Extension: ".jpg",
					Content:   strings.NewReader("test photo"),
				})
				if err != nil {
					t.Fatal(err)
				}
				key = keys[0]
				return key, nil
			}
			var schoolClient StudentPhotoClient
			if !tt.schoolDisabled {
				schoolClient = testStudentPhotoClient(fetchPhoto)
			}
			logger := zap.NewNop()
			dao := mysql.NewDAO(db, logger)
			service := NewRecordService(dao, NewProfileService(dao, storage, schoolClient, logger), logger)
			requester := studentID
			if tt.requestedBy != "" {
				requester = tt.requestedBy
			}
			response, err := service.GetRecord(ctx, requester, recordID)
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
			if key != "" {
				_, err := os.Stat(filepath.Join(storage.RootDir, filepath.FromSlash(key)))
				if (tt.wantFile && err != nil) || (!tt.wantFile && !errors.Is(err, os.ErrNotExist)) {
					t.Fatalf("photo file should exist: %t, stat error: %v", tt.wantFile, err)
				}
			}
		})
	}
}
