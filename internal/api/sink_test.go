package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/koto7/tuma/internal/config"
)

func TestEchoSinkRecordsAndFails(t *testing.T) {
	s := &Server{
		cfg:  &config.Config{MaxBodyBytes: 1 << 20, PlaygroundInternalBase: "http://tuma-api:8080"},
		sink: newEchoSink(),
	}

	req := httptest.NewRequest(http.MethodPost, "/sink/echo", strings.NewReader(`{"ok":true}`))
	req.Header.Set("X-Tuma-Delivery-Id", "del_1")
	rec := httptest.NewRecorder()
	s.sinkReceive(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	s.sink.setFail(true)
	req = httptest.NewRequest(http.MethodPost, "/sink/echo", strings.NewReader(`{"ok":false}`))
	rec = httptest.NewRecorder()
	s.sinkReceive(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}

	fail, events := s.sink.snapshot()
	if !fail || len(events) != 2 {
		t.Fatalf("fail=%v events=%d", fail, len(events))
	}
	if events[0].DeliveryID != "del_1" || events[0].Status != 200 {
		t.Fatalf("first event: %+v", events[0])
	}
	if events[1].Status != 502 {
		t.Fatalf("second status %d", events[1].Status)
	}
	if s.sinkURL() != "http://tuma-api:8080/sink/echo" {
		t.Fatalf("sink url %s", s.sinkURL())
	}
}
