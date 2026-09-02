package speedtest

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

type Run struct {
	ID, Owner string
	Expires   time.Time
	ctx       context.Context
	cancel    context.CancelFunc
	slots     chan struct{}
	once      sync.Once
}

func (r *Run) Context() context.Context { return r.ctx }
func (r *Run) Acquire(ctx context.Context) bool {
	select {
	case <-r.ctx.Done():
		return false
	case <-ctx.Done():
		return false
	case r.slots <- struct{}{}:
		return true
	default:
		return false
	}
}
func (r *Run) Release() {
	select {
	case <-r.slots:
	default:
	}
}
func (r *Run) close() { r.once.Do(func() { r.cancel() }) }

type Registry struct {
	mu                sync.Mutex
	runs              map[string]*Run
	max, per, streams int
	ttl               time.Duration
}

func New(max, per int, args ...any) *Registry {
	streams, ttl := 8, 15*time.Second
	if len(args) == 1 {
		ttl = args[0].(time.Duration)
	}
	if len(args) == 2 {
		streams = args[0].(int)
		ttl = args[1].(time.Duration)
	}
	return &Registry{runs: map[string]*Run{}, max: max, per: per, streams: streams, ttl: ttl}
}
func (r *Registry) Create(owner string) (*Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupLocked()
	if len(r.runs) >= r.max {
		return nil, errors.New("global limit")
	}
	n := 0
	for _, x := range r.runs {
		if x.Owner == owner {
			n++
		}
	}
	if n >= r.per {
		return nil, errors.New("session limit")
	}
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.ttl)
	x := &Run{ID: base64.RawURLEncoding.EncodeToString(b), Owner: owner, Expires: time.Now().Add(r.ttl), ctx: ctx, cancel: cancel, slots: make(chan struct{}, r.streams)}
	r.runs[x.ID] = x
	go func() { <-ctx.Done(); r.Close(x.ID) }()
	return x, nil
}
func (r *Registry) Get(id, owner string) (*Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupLocked()
	x := r.runs[id]
	if x == nil || x.Owner != owner || x.ctx.Err() != nil {
		return nil, errors.New("not found")
	}
	return x, nil
}
func (r *Registry) Close(id string) {
	r.mu.Lock()
	x := r.runs[id]
	delete(r.runs, id)
	r.mu.Unlock()
	if x != nil {
		x.close()
	}
}
func (r *Registry) CloseAll() {
	r.mu.Lock()
	runs := r.runs
	r.runs = map[string]*Run{}
	r.mu.Unlock()
	for _, x := range runs {
		x.close()
	}
}
func (r *Registry) cleanupLocked() {
	for k, x := range r.runs {
		if x.ctx.Err() != nil || time.Now().After(x.Expires) {
			delete(r.runs, k)
			x.close()
		}
	}
}
