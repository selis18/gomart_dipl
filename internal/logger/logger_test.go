package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestLoggerRecordsActualResponse(t *testing.T) {
	core, entries := observer.New(zap.InfoLevel)
	previous := Log
	Log = zap.New(core)
	t.Cleanup(func() { Log = previous })
	for _, explicit := range []bool{false, true} {
		entries.TakeAll()
		recorder := httptest.NewRecorder()
		wantStatus, wantSize := 200, 0
		if explicit {
			wantStatus, wantSize = 500, 4
		}
		RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if explicit {
				w.WriteHeader(500)
				w.Write([]byte("oops"))
				w.WriteHeader(200)
			}
		})).ServeHTTP(recorder, httptest.NewRequest("GET", "/api/user/balance", nil))
		logs := entries.All()
		if len(logs) != 1 {
			t.Fatalf("logs=%v", logs)
		}
		if logs[0].ContextMap()["URI"] != "/api/user/balance" || logs[0].ContextMap()["Method"] != "GET" || logs[0].ContextMap()["Request time"] == nil {
			t.Fatal("lost request fields")
		}
		response := logs[0].ContextMap()
		if response["Status"] != int64(wantStatus) || response["Size"] != int64(wantSize) || recorder.Code != wantStatus {
			t.Fatalf("response=%v actual status=%d", response, recorder.Code)
		}
	}
}
