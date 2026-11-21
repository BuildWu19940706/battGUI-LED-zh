package daemon

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/charlie0129/batt/pkg/config"
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
	f := newFixture(t, firmwareManaged, withFileConfig())
	f.file.SetChargeRange(80, 78)
	f.smc.fail("EnsureFirmwareChargeLimit", errors.New("cannot apply range"))
	blocked := &blockingSaveConfig{Config: f.file, entered: make(chan struct{}), release: make(chan struct{})}
	f.d.conf = blocked
	rangeDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { rangeDone <- f.put("/charge-range", `{"upper":70,"lower":60}`) }()
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		t.Fatal("range did not reach persistence")
	}
	lowerDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { lowerDone <- f.put("/lower-limit-delta", "5") }()
	configDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { configDone <- f.serve(http.MethodGet, "/config", "") }()
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
	if earlyLower != nil || earlyConfig != nil {
		t.Error("a competing request completed before the transaction finished")
	}
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
	stored, err := config.NewFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	if stored.UpperLimit() != 80 || stored.LowerLimit() != 75 {
		t.Fatalf("successful legacy update lost on rollback: %v", stored.LogValue())
	}
}
