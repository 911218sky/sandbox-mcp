package sandbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testSessionConfig(t *testing.T) SessionConfig {
	t.Helper()
	dir := t.TempDir()
	return SessionConfig{
		BaseDir:     dir,
		MaxSessions: 5,
		MaxAge:      100 * time.Millisecond,
		CleanupTick: 50 * time.Millisecond,
	}
}

func TestNewSessionManager(t *testing.T) {
	cfg := testSessionConfig(t)
	sm, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}
	defer sm.Shutdown()

	if _, err := os.Stat(cfg.BaseDir); os.IsNotExist(err) {
		t.Fatal("BaseDir should be created")
	}

	active, max := sm.Stats()
	if active != 0 || max != cfg.MaxSessions {
		t.Fatalf("Stats mismatch: got (%d, %d), want (0, %d)", active, max, cfg.MaxSessions)
	}
}

func TestNewSessionManager_EmptyBaseDir(t *testing.T) {
	sm, err := NewSessionManager(SessionConfig{})
	if err != nil {
		t.Fatalf("NewSessionManager with empty config failed: %v", err)
	}
	defer sm.Shutdown()

	if sm.config.BaseDir != DefaultSessionConfig().BaseDir {
		t.Fatalf("Expected default BaseDir %q, got %q", DefaultSessionConfig().BaseDir, sm.config.BaseDir)
	}
}

func TestGetOrCreate_New(t *testing.T) {
	cfg := testSessionConfig(t)
	sm, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}
	defer sm.Shutdown()

	session, err := sm.GetOrCreate("test-session-1")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	if session.ID != "test-session-1" {
		t.Fatalf("Expected ID 'test-session-1', got %q", session.ID)
	}

	expectedDir := filepath.Join(cfg.BaseDir, "test-session-1")
	if session.Dir != expectedDir {
		t.Fatalf("Expected dir %q, got %q", expectedDir, session.Dir)
	}

	if _, err := os.Stat(session.Dir); os.IsNotExist(err) {
		t.Fatal("Session directory should exist")
	}

	active, _ := sm.Stats()
	if active != 1 {
		t.Fatalf("Expected 1 active session, got %d", active)
	}
}

func TestGetOrCreate_Existing(t *testing.T) {
	cfg := testSessionConfig(t)
	sm, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}
	defer sm.Shutdown()

	session1, err := sm.GetOrCreate("reuse-session")
	if err != nil {
		t.Fatalf("First GetOrCreate failed: %v", err)
	}

	firstLastUsed := session1.LastUsed
	time.Sleep(50 * time.Millisecond)

	session2, err := sm.GetOrCreate("reuse-session")
	if err != nil {
		t.Fatalf("Second GetOrCreate failed: %v", err)
	}

	if session1.ID != session2.ID || session1.Dir != session2.Dir {
		t.Fatal("Should return the same session")
	}

	if !session2.LastUsed.After(firstLastUsed) {
		t.Fatal("LastUsed should be updated on second access")
	}

	active, _ := sm.Stats()
	if active != 1 {
		t.Fatalf("Expected 1 active session, got %d", active)
	}
}

func TestGetOrCreate_LRUEviction(t *testing.T) {
	cfg := testSessionConfig(t)
	cfg.MaxSessions = 3
	sm, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}
	defer sm.Shutdown()

	// Create 3 sessions
	for i := 1; i <= 3; i++ {
		_, err := sm.GetOrCreate(string(rune('A' + i - 1)))
		if err != nil {
			t.Fatalf("GetOrCreate session %d failed: %v", i, err)
		}
	}

	active, _ := sm.Stats()
	if active != 3 {
		t.Fatalf("Expected 3 active sessions, got %d", active)
	}

	// Access session B to make it recently used
	_, _ = sm.GetOrCreate("B")

	// Create 4th session - should evict A (oldest/least recently used)
	_, err = sm.GetOrCreate("D")
	if err != nil {
		t.Fatalf("GetOrCreate D failed: %v", err)
	}

	active, _ = sm.Stats()
	if active != 3 {
		t.Fatalf("Expected 3 active sessions after eviction, got %d", active)
	}

	// A should be evicted (was least recently used)
	if _, exists := sm.sessions["A"]; exists {
		t.Fatal("Session A should have been evicted (LRU)")
	}

	// B, C, D should still exist
	for _, id := range []string{"B", "C", "D"} {
		if _, exists := sm.sessions[id]; !exists {
			t.Fatalf("Session %s should still exist", id)
		}
	}
}

func TestRemove(t *testing.T) {
	cfg := testSessionConfig(t)
	sm, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}
	defer sm.Shutdown()

	session, _ := sm.GetOrCreate("to-remove")
	dir := session.Dir

	// Write a file inside session dir
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)

	err = sm.Remove("to-remove")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("Session directory should be removed")
	}

	active, _ := sm.Stats()
	if active != 0 {
		t.Fatalf("Expected 0 active sessions after remove, got %d", active)
	}
}

func TestRemove_NonExistent(t *testing.T) {
	cfg := testSessionConfig(t)
	sm, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}
	defer sm.Shutdown()

	err = sm.Remove("nonexistent")
	if err != nil {
		t.Fatalf("Remove of nonexistent session should not error, got: %v", err)
	}
}

func TestCleanupExpired(t *testing.T) {
	cfg := testSessionConfig(t)
	cfg.MaxAge = 50 * time.Millisecond
	cfg.CleanupTick = 200 * time.Millisecond // long enough that manual test works
	sm, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}
	defer sm.Shutdown()

	sm.GetOrCreate("expiring-1")
	sm.GetOrCreate("expiring-2")

	// Wait for sessions to expire
	time.Sleep(80 * time.Millisecond)

	// Manually trigger cleanup
	sm.cleanupExpired()

	active, _ := sm.Stats()
	if active != 0 {
		t.Fatalf("Expected 0 active sessions after expiry, got %d", active)
	}
}

func TestCleanupExpired_MixedAges(t *testing.T) {
	cfg := testSessionConfig(t)
	cfg.MaxAge = 80 * time.Millisecond
	sm, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}
	defer sm.Shutdown()

	sm.GetOrCreate("old-session")

	time.Sleep(60 * time.Millisecond)

	// Create a new session (should not expire)
	sm.GetOrCreate("new-session")

	// Wait for old session to expire
	time.Sleep(40 * time.Millisecond)

	sm.cleanupExpired()

	active, _ := sm.Stats()
	if active != 1 {
		t.Fatalf("Expected 1 active session, got %d", active)
	}

	if _, exists := sm.sessions["new-session"]; !exists {
		t.Fatal("new-session should still exist")
	}
}

func TestShutdown(t *testing.T) {
	cfg := testSessionConfig(t)
	sm, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sm.GetOrCreate("s1")
	sm.GetOrCreate("s2")

	err = sm.Shutdown()
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	if _, err := os.Stat(cfg.BaseDir); !os.IsNotExist(err) {
		t.Fatal("BaseDir should be removed after Shutdown")
	}

	active, _ := sm.Stats()
	if active != 0 {
		t.Fatalf("Expected 0 sessions after Shutdown, got %d", active)
	}
}

func TestAutoCleanupGoroutine(t *testing.T) {
	cfg := testSessionConfig(t)
	cfg.MaxAge = 60 * time.Millisecond
	cfg.CleanupTick = 40 * time.Millisecond
	sm, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}
	defer sm.Shutdown()

	sm.GetOrCreate("auto-expire")

	// Wait for auto-cleanup to run (cleanup tick + some buffer)
	time.Sleep(150 * time.Millisecond)

	active, _ := sm.Stats()
	if active != 0 {
		t.Fatalf("Expected 0 sessions after auto-cleanup, got %d", active)
	}
}

func TestConcurrentAccess(t *testing.T) {
	cfg := testSessionConfig(t)
	cfg.MaxSessions = 100
	sm, err := NewSessionManager(cfg)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}
	defer sm.Shutdown()

	done := make(chan bool, 50)
	for i := 0; i < 50; i++ {
		go func(id int) {
			sessionID := string(rune('a' + id%26))
			_, _ = sm.GetOrCreate(sessionID)
			sm.Stats()
			done <- true
		}(i)
	}

	for i := 0; i < 50; i++ {
		<-done
	}

	// Should not panic or deadlock
	active, max := sm.Stats()
	if active > max {
		t.Fatalf("Active sessions (%d) should not exceed max (%d)", active, max)
	}
}
