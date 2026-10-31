package daemon

import (
	"encoding/json"
	"github.com/charlie0129/batt/pkg/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type blockingSaveConfig struct {
	config.Config
	once             sync.Once
	entered, release chan struct{}
}

func (c *blockingSaveConfig) Save() error {
	c.once.Do(func() { close(c.entered); <-c.release })
	return c.Config.Save()
}

func TestRangeRollbackSerializesLegacyWriterAndSnapshot(t *testing.T) {
	useFirmwareLoopState(t, useMockSMC(t), &mockConf{upper: 80, lower: 78})
	file, path := useTempConfig(t)
	blocked := &blockingSaveConfig{Config: file, entered: make(chan struct{}), release: make(chan struct{})}
	conf = blocked
	router := setupRoutes()
	rangeDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { rangeDone <- requestChargeRange(`{"upper":70,"lower":60}`) }()
	<-blocked.entered
	lowerDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPut, "/lower-limit-delta", strings.NewReader("5"))
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		lowerDone <- res
	}()
	configDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/config", nil))
		configDone <- res
	}()
	var earlyLower, earlyConfig *httptest.ResponseRecorder
	select {
	case earlyLower = <-lowerDone:
	case earlyConfig = <-configDone:
	case <-time.After(30 * time.Millisecond):
	}
	close(blocked.release)
	wait := func(ch chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
		select {
		case res := <-ch:
			return res
		case <-time.After(time.Second):
			t.Fatal("range transaction or concurrent request deadlocked")
			return nil
		}
	}
	rangeResult := wait(rangeDone)
	if earlyLower == nil {
		earlyLower = wait(lowerDone)
	}
	if earlyConfig == nil {
		earlyConfig = wait(configDone)
	}
	if earlyLower.Code != 201 {
		t.Fatalf("legacy update failed: %d %s", earlyLower.Code, earlyLower.Body)
	}
	if rangeResult.Code != 500 {
		t.Fatalf("range must fail hardware application: %d %s", rangeResult.Code, rangeResult.Body)
	}
	res := earlyConfig
	var raw config.RawFileConfig
	if err := json.Unmarshal(res.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Limit == nil || *raw.Limit != 80 {
		t.Fatalf("snapshot exposed provisional range: %s", res.Body)
	}
	stored, err := config.NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if stored.UpperLimit() != 80 || stored.LowerLimit() != 75 {
		t.Fatalf("successful legacy update lost on rollback: %v", stored.LogrusFields())
	}
}
