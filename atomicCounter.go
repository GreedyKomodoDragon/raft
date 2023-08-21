package raft

import (
	"sync"
)

type AtomicCounter struct {
	value uint32
	ids   []uint64
	lock  *sync.RWMutex
}

func NewAtomicCounter() *AtomicCounter {
	return &AtomicCounter{
		value: 0,
		ids:   []uint64{},
		lock:  &sync.RWMutex{},
	}
}

func (c *AtomicCounter) Increment(id uint64) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.value++
	c.ids = append(c.ids, id)
}

func (c *AtomicCounter) GetValue() uint32 {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.value
}

func (c *AtomicCounter) AddIdOnly(id uint64) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.ids = append(c.ids, id)
}

func (c *AtomicCounter) HasId(id uint64) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	for _, value := range c.ids {
		if value == id {
			return true
		}
	}

	return false
}

func (c *AtomicCounter) IdCount() int {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return len(c.ids)
}
