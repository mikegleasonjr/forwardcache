package forwardcache

import (
	"sync"
)

// BufferPool uses sync.Pool for getting and returning temporary byte slices.
type BufferPool struct {
	p *sync.Pool
}

// NewBufferPool creates a new BufferPool.
func NewBufferPool(bufSize int) *BufferPool {
	return &BufferPool{
		&sync.Pool{
			New: func() interface{} { return make([]byte, bufSize) },
		},
	}
}

// DefaultBufferPool is a pool which produces 32k buffers.
var DefaultBufferPool = NewBufferPool(32 * 1024)

// Get gets a buffer from the pool.
func (p *BufferPool) Get() []byte {
	return p.p.Get().([]byte)
}

// Put puts back a buffer to the pool.
func (p *BufferPool) Put(b []byte) {
	p.p.Put(b)
}
