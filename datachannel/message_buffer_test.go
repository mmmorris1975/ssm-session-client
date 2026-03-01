package datachannel

import (
	"sync"
	"testing"
)

func newTestMsg(seq int64) *AgentMessage {
	msg := NewAgentMessage()
	msg.SequenceNumber = seq
	msg.MessageType = InputStreamData
	msg.Payload = []byte("test")
	return msg
}

func TestNewMessageBuffer(t *testing.T) {
	mb := NewMessageBuffer(10)
	if mb == nil {
		t.Fatal("NewMessageBuffer returned nil")
	}
	if mb.Len() != 0 {
		t.Errorf("Len() = %d, want 0", mb.Len())
	}
	if mb.size != 10 {
		t.Errorf("size = %d, want 10", mb.size)
	}
}

func TestAdd(t *testing.T) {
	mb := NewMessageBuffer(5)

	for i := int64(0); i < 5; i++ {
		if err := mb.Add(newTestMsg(i)); err != nil {
			t.Fatalf("Add(seq=%d) error: %v", i, err)
		}
	}

	if mb.Len() != 5 {
		t.Errorf("Len() = %d, want 5", mb.Len())
	}
}

func TestAdd_BufferFull(t *testing.T) {
	mb := NewMessageBuffer(2)

	if err := mb.Add(newTestMsg(0)); err != nil {
		t.Fatalf("Add(0) error: %v", err)
	}
	if err := mb.Add(newTestMsg(1)); err != nil {
		t.Fatalf("Add(1) error: %v", err)
	}

	err := mb.Add(newTestMsg(2))
	if err != ErrBufferFull {
		t.Errorf("Add(2) error = %v, want ErrBufferFull", err)
	}
}

func TestGet(t *testing.T) {
	mb := NewMessageBuffer(10)

	msg := newTestMsg(42)
	if err := mb.Add(msg); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	got := mb.Get(42)
	if got == nil {
		t.Fatal("Get(42) returned nil")
	}
	if got.SequenceNumber != 42 {
		t.Errorf("Get(42).SequenceNumber = %d, want 42", got.SequenceNumber)
	}
}

func TestGet_NotFound(t *testing.T) {
	mb := NewMessageBuffer(10)

	got := mb.Get(99)
	if got != nil {
		t.Errorf("Get(99) = %v, want nil", got)
	}
}

func TestRemove(t *testing.T) {
	mb := NewMessageBuffer(10)

	if err := mb.Add(newTestMsg(1)); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if err := mb.Add(newTestMsg(2)); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	mb.Remove(1)

	if mb.Len() != 1 {
		t.Errorf("Len() = %d, want 1", mb.Len())
	}
	if mb.Get(1) != nil {
		t.Error("Get(1) should be nil after Remove")
	}
	if mb.Get(2) == nil {
		t.Error("Get(2) should not be nil")
	}
}

func TestRemove_NonExistent(t *testing.T) {
	mb := NewMessageBuffer(10)

	// Should not panic
	mb.Remove(99)
}

func TestNext(t *testing.T) {
	mb := NewMessageBuffer(10)

	if err := mb.Add(newTestMsg(1)); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if err := mb.Add(newTestMsg(2)); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if err := mb.Add(newTestMsg(3)); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	msg1 := mb.Next()
	if msg1 == nil || msg1.SequenceNumber != 1 {
		t.Errorf("Next() #1 = %v, want seq 1", msg1)
	}

	msg2 := mb.Next()
	if msg2 == nil || msg2.SequenceNumber != 2 {
		t.Errorf("Next() #2 = %v, want seq 2", msg2)
	}

	msg3 := mb.Next()
	if msg3 == nil || msg3.SequenceNumber != 3 {
		t.Errorf("Next() #3 = %v, want seq 3", msg3)
	}

	// Should return nil after exhausting
	msg4 := mb.Next()
	if msg4 != nil {
		t.Errorf("Next() #4 = %v, want nil", msg4)
	}
}

func TestNext_EmptyBuffer(t *testing.T) {
	mb := NewMessageBuffer(10)

	msg := mb.Next()
	if msg != nil {
		t.Errorf("Next() on empty buffer = %v, want nil", msg)
	}
}

func TestAddAfterRemove_FreesSpace(t *testing.T) {
	mb := NewMessageBuffer(2)

	if err := mb.Add(newTestMsg(1)); err != nil {
		t.Fatalf("Add(1) error: %v", err)
	}
	if err := mb.Add(newTestMsg(2)); err != nil {
		t.Fatalf("Add(2) error: %v", err)
	}

	mb.Remove(1)

	// Should be able to add again after removing
	if err := mb.Add(newTestMsg(3)); err != nil {
		t.Fatalf("Add(3) after Remove error: %v", err)
	}

	if mb.Len() != 2 {
		t.Errorf("Len() = %d, want 2", mb.Len())
	}
}

func TestConcurrentAccess(t *testing.T) {
	mb := NewMessageBuffer(100)

	var wg sync.WaitGroup

	// Add messages concurrently
	for i := int64(0); i < 50; i++ {
		wg.Add(1)
		go func(seq int64) {
			defer wg.Done()
			_ = mb.Add(newTestMsg(seq))
		}(i)
	}

	// Get messages concurrently
	for i := int64(0); i < 50; i++ {
		wg.Add(1)
		go func(seq int64) {
			defer wg.Done()
			mb.Get(seq)
		}(i)
	}

	wg.Wait()

	if mb.Len() != 50 {
		t.Errorf("Len() = %d, want 50", mb.Len())
	}
}

func TestConcurrentAddAndRemove(t *testing.T) {
	mb := NewMessageBuffer(100)

	// Pre-fill
	for i := int64(0); i < 50; i++ {
		if err := mb.Add(newTestMsg(i)); err != nil {
			t.Fatalf("Add(%d) error: %v", i, err)
		}
	}

	var wg sync.WaitGroup

	// Remove even, add new
	for i := int64(0); i < 25; i++ {
		wg.Add(2)
		go func(seq int64) {
			defer wg.Done()
			mb.Remove(seq * 2)
		}(i)
		go func(seq int64) {
			defer wg.Done()
			_ = mb.Add(newTestMsg(50 + seq))
		}(i)
	}

	wg.Wait()

	// Should have 50 messages (removed 25, added 25)
	if mb.Len() != 50 {
		t.Errorf("Len() = %d, want 50", mb.Len())
	}
}

func TestZeroSizeBuffer(t *testing.T) {
	mb := NewMessageBuffer(0)

	err := mb.Add(newTestMsg(0))
	if err != ErrBufferFull {
		t.Errorf("Add to zero-size buffer: error = %v, want ErrBufferFull", err)
	}
}
