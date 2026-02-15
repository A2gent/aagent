package speechcache

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const DefaultTTL = 15 * time.Minute

type clip struct {
	contentType string
	data        []byte
	createdAt   time.Time
}

// Store keeps short-lived generated speech clips in memory for web playback.
type Store struct {
	mu    sync.Mutex
	ttl   time.Duration
	clips map[string]clip
}

func New(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Store{
		ttl:   ttl,
		clips: make(map[string]clip, 32),
	}
}

func (s *Store) Save(contentType string, data []byte) string {
	if s == nil {
		return ""
	}

	ct := strings.TrimSpace(contentType)
	if ct == "" {
		ct = "audio/mpeg"
	}
	payload := make([]byte, len(data))
	copy(payload, data)

	id := uuid.New().String()
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked(now)
	s.clips[id] = clip{
		contentType: ct,
		data:        payload,
		createdAt:   now,
	}
	return id
}

func (s *Store) Load(id string) (string, []byte, bool) {
	if s == nil {
		return "", nil, false
	}

	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked(now)

	item, ok := s.clips[strings.TrimSpace(id)]
	if !ok {
		return "", nil, false
	}

	payload := make([]byte, len(item.data))
	copy(payload, item.data)
	return item.contentType, payload, true
}

func (s *Store) cleanupExpiredLocked(now time.Time) {
	cutoff := now.Add(-s.ttl)
	for id, item := range s.clips {
		if item.createdAt.Before(cutoff) {
			delete(s.clips, id)
		}
	}
}
