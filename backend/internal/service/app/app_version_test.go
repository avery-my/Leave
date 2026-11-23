package app

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"backend/internal/dto"
	"backend/internal/response"
	"backend/internal/service/common/releasenotes"
	"github.com/gin-gonic/gin"
)

func TestAppVersionResponseContracts(t *testing.T) {
	for _, raw := range []string{`["旧说明"]`, `{"features":["新增"],"notices":["重新登录"]}`} {
		notes, grouped, err := releasenotes.ParseReleaseNotes(raw)
		if err != nil {
			t.Fatal(err)
		}
		if grouped != nil {
			notes = nil
		}
		version := dto.AppVersionResponse{
			Platform: "android", LatestVersionCode: 17, LatestVersionName: "1.2.0",
			Updates: []dto.AppVersionUpdate{{VersionCode: 17, VersionName: "1.2.0", UncategorizedNotes: notes, ReleaseNotesGrouped: grouped}},
		}
		for _, forced := range []bool{false, true} {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			if forced {
				response.ErrorWithData(context, response.CodeForceUpdate, version)
			} else {
				response.Success(context, version)
			}
			var wire struct {
				Code response.Code          `json:"code"`
				Data dto.AppVersionResponse `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &wire); err != nil {
				t.Fatal(err)
			}
			if len(wire.Data.Updates) != 1 || !reflect.DeepEqual(wire.Data.Updates[0].UncategorizedNotes, notes) || !reflect.DeepEqual(wire.Data.Updates[0].ReleaseNotesGrouped, grouped) {
				t.Fatalf("forced=%v response lost notes: %s", forced, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), `"release_notes":`) {
				t.Fatalf("forced=%v returned legacy field: %s", forced, recorder.Body.String())
			}
			if forced && wire.Code != response.CodeForceUpdate {
				t.Fatalf("wrong forced update code: %v", wire.Code)
			}
		}
	}
}
