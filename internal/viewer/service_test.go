package viewer

import (
	"os"
	"path/filepath"
	"testing"

	"ccecho/internal/config"
	"ccecho/internal/sessionmeta"
)

func TestSessionItemsSortByNumericIdx(t *testing.T) {
	logDir := t.TempDir()
	sessionName := "session-a"
	sessionDir := filepath.Join(logDir, sessionName)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}

	err := sessionmeta.Write(sessionDir, sessionmeta.Meta{
		SessionName:  sessionName,
		SessionPath:  sessionDir,
		Provider:     config.ProviderCodex,
		Target:       "test-target",
		ProxyAddr:    "127.0.0.1:0",
		LocalBaseURL: "http://localhost",
		CreatedAt:    "2026-03-27T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("write meta: %v", err)
	}

	requestBody := []byte(`{"model":"gpt-test","input":[]}`)
	for _, name := range []string{"request11.json", "request3.json", "request1.json"} {
		if err := os.WriteFile(filepath.Join(sessionDir, name), requestBody, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	service := NewService(logDir)
	items := service.sessionItems(sessionName)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	got := []int{items[0].Idx, items[1].Idx, items[2].Idx}
	want := []int{1, 3, 11}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected order: got %v want %v", got, want)
		}
	}
}
