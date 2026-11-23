package app

import (
	"net/http/httptest"
	"testing"

	"backend/internal/dao/mysql"
	"backend/internal/request"
	appservice "backend/internal/service/app"
	sharedoptions "backend/internal/service/common/options"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestApartmentOptionsRejectInvalidGender(t *testing.T) {
	dao := mysql.NewDAO(nil, zap.NewNop())
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(request.CtxStuID, "test") })
	r.GET("/apartments", NewOptionsHandler(appservice.NewOptionsService(dao, sharedoptions.NewOptionsService(dao, zap.NewNop()), zap.NewNop()), sharedoptions.NewOptionsService(dao, zap.NewNop()), zap.NewNop()).GetApartmentOptionsHandler)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/apartments?gender=unknown", nil))
	if w.Code != 400 {
		t.Fatalf("invalid gender status: %d", w.Code)
	}
}

func TestProfileOptionsRequireStudentIdentity(t *testing.T) {
	dao := mysql.NewDAO(nil, zap.NewNop())
	optionsHandler := NewOptionsHandler(appservice.NewOptionsService(dao, sharedoptions.NewOptionsService(dao, zap.NewNop()), zap.NewNop()), sharedoptions.NewOptionsService(dao, zap.NewNop()), zap.NewNop())
	for _, handler := range []gin.HandlerFunc{optionsHandler.GetClassOptionsHandler, optionsHandler.GetApartmentOptionsHandler} {
		r := gin.New()
		r.GET("/", handler)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != 401 {
			t.Fatalf("missing identity status: %d", w.Code)
		}
	}
}
