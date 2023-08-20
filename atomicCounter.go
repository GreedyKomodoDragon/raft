package raft

import "sync/atomic"

type AtomicCounter struct {
	value uint32
}

func (c *AtomicCounter) Increment() {
	atomic.AddUint32(&c.value, 1)
}

func (c *AtomicCounter) Decrement() {
	atomic.AddUint32(&c.value, ^uint32(0)) // Subtract 1 using two's complement
}

func (c *AtomicCounter) GetValue() uint32 {
	return atomic.LoadUint32(&c.value)
}
