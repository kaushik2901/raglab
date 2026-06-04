package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRingBuffer(t *testing.T) {
	r := NewRingBuffer(5)
	assert.NotNil(t, r)
	assert.Equal(t, 5, r.maxTurns)
	assert.NotNil(t, r.buffer)
}

func TestNewRingBufferDefault(t *testing.T) {
	r := NewRingBuffer(0)
	assert.Equal(t, 10, r.maxTurns)
}

func TestAddAndGet(t *testing.T) {
	r := NewRingBuffer(3)

	r.Add("conv1", "hello", "hi there")
	r.Add("conv1", "how are you?", "I'm good")

	turns := r.Get("conv1")
	assert.Len(t, turns, 2)
	assert.Equal(t, "user", turns[0].User.Role)
	assert.Equal(t, "hello", turns[0].User.Content)
	assert.Equal(t, "assistant", turns[0].Assistant.Role)
	assert.Equal(t, "hi there", turns[0].Assistant.Content)
	assert.Equal(t, "how are you?", turns[1].User.Content)
}

func TestRingBufferEviction(t *testing.T) {
	r := NewRingBuffer(2)

	r.Add("conv1", "q1", "a1")
	r.Add("conv1", "q2", "a2")
	r.Add("conv1", "q3", "a3")

	turns := r.Get("conv1")
	assert.Len(t, turns, 2)
	assert.Equal(t, "q2", turns[0].User.Content)
	assert.Equal(t, "q3", turns[1].User.Content)
}

func TestGetIsolatedConversations(t *testing.T) {
	r := NewRingBuffer(10)

	r.Add("conv1", "q1", "a1")
	r.Add("conv2", "q2", "a2")

	turns1 := r.Get("conv1")
	assert.Len(t, turns1, 1)
	assert.Equal(t, "q1", turns1[0].User.Content)

	turns2 := r.Get("conv2")
	assert.Len(t, turns2, 1)
	assert.Equal(t, "q2", turns2[0].User.Content)
}

func TestClear(t *testing.T) {
	r := NewRingBuffer(10)
	r.Add("conv1", "q1", "a1")
	assert.Len(t, r.Get("conv1"), 1)

	r.Clear("conv1")
	assert.Len(t, r.Get("conv1"), 0)
}

func TestGetEmptyConversation(t *testing.T) {
	r := NewRingBuffer(10)
	turns := r.Get("nonexistent")
	assert.Len(t, turns, 0)
}

func TestGetReturnsCopy(t *testing.T) {
	r := NewRingBuffer(10)
	r.Add("conv1", "q1", "a1")

	turns := r.Get("conv1")
	turns[0].User.Content = "modified"

	original := r.Get("conv1")
	assert.Equal(t, "q1", original[0].User.Content)
}

func TestAddConcurrentSafe(t *testing.T) {
	r := NewRingBuffer(100)
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			r.Add("conv1", "q", "a")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			r.Get("conv1")
		}
		done <- true
	}()

	<-done
	<-done
	assert.Len(t, r.Get("conv1"), 100)
}
