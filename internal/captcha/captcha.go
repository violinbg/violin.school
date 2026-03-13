// Package captcha provides a simple server-side math captcha for registration.
package captcha

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

const ttl = 10 * time.Minute

type entry struct {
	answer    int
	expiresAt time.Time
}

// Store holds pending captcha challenges in memory.
type Store struct {
	mu      sync.Mutex
	entries map[string]entry
}

// NewStore creates a new captcha Store.
func NewStore() *Store {
	return &Store{entries: make(map[string]entry)}
}

// Generate creates a new math captcha challenge and returns its ID and question string.
func (s *Store) Generate() (id, question string) {
	a := rand.Intn(15) + 1 //nolint:gosec
	b := rand.Intn(15) + 1 //nolint:gosec
	id = uuid.New().String()
	question = fmt.Sprintf("%d + %d", a, b)
	s.mu.Lock()
	s.entries[id] = entry{answer: a + b, expiresAt: time.Now().Add(ttl)}
	s.mu.Unlock()
	s.cleanup()
	return id, question
}

// Verify checks whether the submitted answer matches the stored challenge.
// The entry is deleted on the first verification attempt (pass or fail) to prevent replay.
func (s *Store) Verify(id, answer string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return false
	}
	delete(s.entries, id)
	if time.Now().After(e.expiresAt) {
		return false
	}
	submitted, err := strconv.Atoi(answer)
	if err != nil {
		return false
	}
	return submitted == e.answer
}

// cleanup removes expired entries without holding the outer lock long.
func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, e := range s.entries {
		if now.After(e.expiresAt) {
			delete(s.entries, id)
		}
	}
}
