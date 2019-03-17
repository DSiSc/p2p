package common

import (
	"container/list"
	"github.com/DSiSc/craft/types"
	"sync"
)

// ringRecord represent a record in ring buffer
type ringRecord struct {
	v    interface{}
	node *list.Element
}

// RingBuffer is a ring buffer implementation.
type RingBuffer struct {
	elements map[types.Hash]*ringRecord
	limit    int
	keyList  *list.List
	lock     sync.RWMutex
}

// NewRingBuffer create a ring buffer instance
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		elements: make(map[types.Hash]*ringRecord),
		limit:    size,
		keyList:  list.New(),
	}
}

// AddElement add a element to ring buffer
func (ring *RingBuffer) AddElement(hash types.Hash, elem interface{}) {
	ring.lock.Lock()
	defer ring.lock.Unlock()
	if r, ok := ring.elements[hash]; ok {
		ring.keyList.Remove(r.node)
	}
	first := ring.keyList.PushFront(hash)
	ring.elements[hash] = &ringRecord{
		v:    elem,
		node: first,
	}
	if len(ring.elements) > ring.limit {
		last := ring.keyList.Back()
		delete(ring.elements, last.Value.(types.Hash))
		ring.keyList.Remove(last)
	}
}

// Exist check if the element is already in the ring buffer
func (ring *RingBuffer) Exist(hash types.Hash) bool {
	ring.lock.RLock()
	defer ring.lock.RUnlock()
	return nil != ring.elements[hash]
}
