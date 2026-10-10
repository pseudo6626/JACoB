package tabstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const maxBatchOperations = 128

type BatchOperation struct {
	Op    string `json:"op"`
	Key   string `json:"key,omitempty"`
	Value any    `json:"value,omitempty"`
}

type BatchResult struct {
	Applied int `json:"applied"`
}

type CompareSetResult struct {
	Swapped bool `json:"swapped"`
	Found   bool `json:"found"`
	Value   any  `json:"value,omitempty"`
}

func (s *Store) CompareSet(tabID, key string, expectedFound bool, expected, value any) (CompareSetResult, error) {
	if !validID(tabID) {
		return CompareSetResult{}, errors.New("invalid tab id")
	}
	if !validKey(key) {
		return CompareSetResult{}, errors.New("state key is required and must be 128 bytes or fewer")
	}
	if err := validateStateValue(value); err != nil {
		return CompareSetResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	bucket := s.tabs[tabID]
	current, found := any(nil), false
	if bucket != nil {
		current, found = bucket[key]
	}
	if found != expectedFound || (found && !jsonEqual(current, expected)) {
		return CompareSetResult{Swapped: false, Found: found, Value: clone(current)}, nil
	}

	old := cloneBucket(bucket)
	if s.tabs[tabID] == nil {
		s.tabs[tabID] = map[string]any{}
	}
	s.tabs[tabID][key] = clone(value)
	if err := s.checkTabLimitLocked(tabID); err != nil {
		restoreBucket(s, tabID, old)
		return CompareSetResult{}, err
	}
	if err := s.persistLocked(); err != nil {
		restoreBucket(s, tabID, old)
		return CompareSetResult{}, err
	}
	return CompareSetResult{Swapped: true, Found: true, Value: clone(value)}, nil
}

func (s *Store) Batch(tabID string, ops []BatchOperation) (BatchResult, error) {
	if !validID(tabID) {
		return BatchResult{}, errors.New("invalid tab id")
	}
	if len(ops) == 0 {
		return BatchResult{}, nil
	}
	if len(ops) > maxBatchOperations {
		return BatchResult{}, fmt.Errorf("state batch contains %d operations; maximum is %d", len(ops), maxBatchOperations)
	}

	for i, op := range ops {
		switch strings.ToLower(strings.TrimSpace(op.Op)) {
		case "set":
			if !validKey(op.Key) {
				return BatchResult{}, fmt.Errorf("operation %d has an invalid state key", i)
			}
			if err := validateStateValue(op.Value); err != nil {
				return BatchResult{}, fmt.Errorf("operation %d: %w", i, err)
			}
		case "delete":
			if !validKey(op.Key) {
				return BatchResult{}, fmt.Errorf("operation %d has an invalid state key", i)
			}
		default:
			return BatchResult{}, fmt.Errorf("operation %d has unsupported op %q", i, op.Op)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	old := cloneBucket(s.tabs[tabID])
	if s.tabs[tabID] == nil {
		s.tabs[tabID] = map[string]any{}
	}
	for _, op := range ops {
		switch strings.ToLower(strings.TrimSpace(op.Op)) {
		case "set":
			s.tabs[tabID][op.Key] = clone(op.Value)
		case "delete":
			delete(s.tabs[tabID], op.Key)
		}
	}
	if len(s.tabs[tabID]) == 0 {
		delete(s.tabs, tabID)
	}
	if _, ok := s.tabs[tabID]; ok {
		if err := s.checkTabLimitLocked(tabID); err != nil {
			restoreBucket(s, tabID, old)
			return BatchResult{}, err
		}
	}
	if err := s.persistLocked(); err != nil {
		restoreBucket(s, tabID, old)
		return BatchResult{}, err
	}
	return BatchResult{Applied: len(ops)}, nil
}

func validateStateValue(value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode state value: %w", err)
	}
	if len(b) > maxValueBytes {
		return fmt.Errorf("state value exceeds %d byte limit", maxValueBytes)
	}
	return nil
}

func jsonEqual(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ab, bb)
}

func restoreBucket(s *Store, tabID string, old map[string]any) {
	if old == nil {
		delete(s.tabs, tabID)
		return
	}
	s.tabs[tabID] = old
}
