package agent

import "sync"

// IterationBudget tracks a shared iteration budget across parent and child agents.
type IterationBudget struct {
	mu       sync.Mutex
	maxTotal int
	used     int
}

// NewIterationBudget creates a new budget with a maximum total.
func NewIterationBudget(maxTotal int) *IterationBudget {
	return &IterationBudget{maxTotal: maxTotal}
}

// Consume tries to use one iteration. Returns true if allowed.
func (b *IterationBudget) Consume() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used >= b.maxTotal {
		return false
	}
	b.used++
	return true
}

// Refund returns one iteration (e.g., for execute_code).
func (b *IterationBudget) Refund() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used > 0 {
		b.used--
	}
}

// Used returns the number of iterations consumed.
func (b *IterationBudget) Used() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

// Remaining returns the iterations left.
func (b *IterationBudget) Remaining() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.maxTotal - b.used
}
