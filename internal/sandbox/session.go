package sandbox

import (
	"container/list"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionConfig represents session management configuration
type SessionConfig struct {
	BaseDir      string        // Base directory for sessions
	MaxSessions  int           // Maximum number of active sessions (LRU eviction)
	MaxAge       time.Duration // Session expiration time
	CleanupTick  time.Duration // How often to run cleanup
}

// DefaultSessionConfig returns default configuration
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		BaseDir:     "/tmp/sandbox-mcp-sessions",
		MaxSessions: 50,              // Max 50 concurrent sessions
		MaxAge:      2 * time.Hour,   // 2 hours inactivity
		CleanupTick: 10 * time.Minute, // Clean up every 10 minutes
	}
}

// SessionManager manages persistent session directories with LRU eviction
type SessionManager struct {
	config     SessionConfig
	mu         sync.RWMutex
	sessions   map[string]*sessionEntry
	lru        *list.List        // LRU list for eviction
	stopCh     chan struct{}     // Stop cleanup goroutine
	wg         sync.WaitGroup
}

type sessionEntry struct {
	session *Session
	elem    *list.Element
}

// Session represents a persistent sandbox session
type Session struct {
	ID        string
	Dir       string
	CreatedAt time.Time
	LastUsed  time.Time
}

// NewSessionManager creates a new session manager with automatic cleanup
func NewSessionManager(config SessionConfig) (*SessionManager, error) {
	if config.BaseDir == "" {
		config = DefaultSessionConfig()
	}
	
	if err := os.MkdirAll(config.BaseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session base directory: %w", err)
	}
	
	sm := &SessionManager{
		config:   config,
		sessions: make(map[string]*sessionEntry),
		lru:      list.New(),
		stopCh:   make(chan struct{}),
	}
	
	// Start background cleanup goroutine
	sm.wg.Add(1)
	go sm.cleanupLoop()
	
	return sm, nil
}

// GetOrCreate gets an existing session or creates a new one
func (sm *SessionManager) GetOrCreate(sessionID string) (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	// Check if session exists
	if entry, exists := sm.sessions[sessionID]; exists {
		entry.session.LastUsed = time.Now()
		// Move to front of LRU (most recently used)
		sm.lru.MoveToFront(entry.elem)
		return entry.session, nil
	}
	
	// Evict oldest session if at capacity
	if len(sm.sessions) >= sm.config.MaxSessions {
		sm.evictOldest()
	}
	
	// Create new session
	sessionDir := filepath.Join(sm.config.BaseDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0777); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}
	
	session := &Session{
		ID:        sessionID,
		Dir:       sessionDir,
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
	}
	
	// Add to LRU
	elem := sm.lru.PushFront(session)
	sm.sessions[sessionID] = &sessionEntry{
		session: session,
		elem:    elem,
	}
	
	return session, nil
}

// Remove removes a session and its directory
func (sm *SessionManager) Remove(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	entry, exists := sm.sessions[sessionID]
	if exists {
		sm.lru.Remove(entry.elem)
		delete(sm.sessions, sessionID)
	}
	
	// Remove directory regardless
	sessionDir := filepath.Join(sm.config.BaseDir, sessionID)
	if _, err := os.Stat(sessionDir); err == nil {
		return os.RemoveAll(sessionDir)
	}
	return nil
}

// evictOldest removes the least recently used session (must be called with lock held)
func (sm *SessionManager) evictOldest() {
	elem := sm.lru.Back()
	if elem == nil {
		return
	}
	
	session := elem.Value.(*Session)
	sm.lru.Remove(elem)
	delete(sm.sessions, session.ID)
	os.RemoveAll(session.Dir)
}

// cleanupLoop runs periodic cleanup of expired sessions
func (sm *SessionManager) cleanupLoop() {
	defer sm.wg.Done()
	
	ticker := time.NewTicker(sm.config.CleanupTick)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			sm.cleanupExpired()
		case <-sm.stopCh:
			return
		}
	}
}

// cleanupExpired removes expired sessions
func (sm *SessionManager) cleanupExpired() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	now := time.Now()
	var toRemove []*list.Element
	
	// Find expired sessions
	for elem := sm.lru.Front(); elem != nil; elem = elem.Next() {
		session := elem.Value.(*Session)
		if now.Sub(session.LastUsed) > sm.config.MaxAge {
			toRemove = append(toRemove, elem)
		}
	}
	
	// Remove expired sessions
	for _, elem := range toRemove {
		session := elem.Value.(*Session)
		sm.lru.Remove(elem)
		delete(sm.sessions, session.ID)
		os.RemoveAll(session.Dir)
	}
}

// Shutdown stops the cleanup goroutine and removes all sessions
func (sm *SessionManager) Shutdown() error {
	close(sm.stopCh)
	sm.wg.Wait()
	
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	// Clear all sessions
	for id := range sm.sessions {
		delete(sm.sessions, id)
	}
	sm.lru.Init()
	
	// Remove base directory
	return os.RemoveAll(sm.config.BaseDir)
}

// Stats returns current session statistics
func (sm *SessionManager) Stats() (int, int) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions), sm.config.MaxSessions
}
