package agent

import (
	"sync"
	"testing"
)

func TestIterationBudget(t *testing.T) {
	b := NewIterationBudget(5)

	if b.Remaining() != 5 {
		t.Errorf("Expected 5 remaining, got %d", b.Remaining())
	}

	if b.Used() != 0 {
		t.Errorf("Expected 0 used, got %d", b.Used())
	}

	// Consume 3
	for i := 0; i < 3; i++ {
		if !b.Consume() {
			t.Errorf("Expected consume to succeed at iteration %d", i)
		}
	}

	if b.Used() != 3 {
		t.Errorf("Expected 3 used, got %d", b.Used())
	}

	if b.Remaining() != 2 {
		t.Errorf("Expected 2 remaining, got %d", b.Remaining())
	}

	// Refund 1
	b.Refund()
	if b.Used() != 2 {
		t.Errorf("Expected 2 used after refund, got %d", b.Used())
	}

	// Consume remaining
	for i := 0; i < 3; i++ {
		b.Consume()
	}

	// Should be exhausted
	if b.Consume() {
		t.Error("Expected consume to fail when budget exhausted")
	}
}

func TestIterationBudgetConcurrent(t *testing.T) {
	b := NewIterationBudget(100)

	var wg sync.WaitGroup
	consumed := make(chan bool, 200)

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consumed <- b.Consume()
		}()
	}

	wg.Wait()
	close(consumed)

	successCount := 0
	for result := range consumed {
		if result {
			successCount++
		}
	}

	if successCount != 100 {
		t.Errorf("Expected exactly 100 successful consumes, got %d", successCount)
	}
}
