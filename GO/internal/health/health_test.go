package health

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewTrackerNonNil(t *testing.T) {
	tr := NewTracker()
	if tr == nil {
		t.Fatal("NewTracker() returned nil")
	}
}

func TestSetRunning(t *testing.T) {
	tr := NewTracker()

	tr.SetRunning(true)
	tr.mu.RLock()
	if !tr.running {
		t.Error("running should be true after SetRunning(true)")
	}
	tr.mu.RUnlock()

	tr.SetRunning(false)
	tr.mu.RLock()
	if tr.running {
		t.Error("running should be false after SetRunning(false)")
	}
	tr.mu.RUnlock()
}

func TestRecordSync(t *testing.T) {
	tr := NewTracker()
	dur := 5 * time.Second
	before := time.Now()
	tr.RecordSync(10, 20, 3, dur)
	after := time.Now()

	tr.mu.RLock()
	defer tr.mu.RUnlock()

	if tr.downloaded != 10 {
		t.Errorf("downloaded = %d, want 10", tr.downloaded)
	}
	if tr.skipped != 20 {
		t.Errorf("skipped = %d, want 20", tr.skipped)
	}
	if tr.failed != 3 {
		t.Errorf("failed = %d, want 3", tr.failed)
	}
	if tr.duration != dur {
		t.Errorf("duration = %v, want %v", tr.duration, dur)
	}
	if tr.lastSync.Before(before) || tr.lastSync.After(after) {
		t.Errorf("lastSync = %v, should be between %v and %v", tr.lastSync, before, after)
	}
}

func buildMux(tr *Tracker) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/sync/status", func(w http.ResponseWriter, r *http.Request) {
		tr.mu.RLock()
		defer tr.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"running":    tr.running,
			"last_sync":  tr.lastSync,
			"downloaded": tr.downloaded,
			"skipped":    tr.skipped,
			"failed":     tr.failed,
			"duration":   tr.duration.String(),
		})
	})
	return mux
}

func TestHealthzEndpoint(t *testing.T) {
	tr := NewTracker()
	mux := buildMux(tr)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestSyncStatusEndpoint(t *testing.T) {
	tr := NewTracker()
	tr.SetRunning(true)
	tr.RecordSync(5, 15, 2, 3*time.Second)

	mux := buildMux(tr)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/sync/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if body["running"] != true {
		t.Errorf("running = %v, want true", body["running"])
	}
	if body["downloaded"] != float64(5) {
		t.Errorf("downloaded = %v, want 5", body["downloaded"])
	}
	if body["skipped"] != float64(15) {
		t.Errorf("skipped = %v, want 15", body["skipped"])
	}
	if body["failed"] != float64(2) {
		t.Errorf("failed = %v, want 2", body["failed"])
	}
	if body["duration"] != "3s" {
		t.Errorf("duration = %v, want %q", body["duration"], "3s")
	}
	// Verify last_sync is present
	if body["last_sync"] == nil {
		t.Error("last_sync should be present")
	}
}

func TestServeContextCancellation(t *testing.T) {
	tr := NewTracker()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Serve(ctx, "127.0.0.1:0", tr, log)
		close(done)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Server shut down gracefully
	case <-time.After(10 * time.Second):
		t.Error("Serve did not shut down after context cancellation")
	}
}
