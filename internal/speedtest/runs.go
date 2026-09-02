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
	mu        sync.Mutex
	closed    bool
	active    sync.WaitGroup
	drained   chan struct{}
}

func (r *Run) Context() context.Context { return r.ctx }
func (r *Run) Acquire(ctx context.Context) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.ctx.Err() != nil {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case r.slots <- struct{}{}:
		r.active.Add(1)
		return true
	default:
		return false
	}
}
func (r *Run) Release() {
	select {
	case <-r.slots:
		r.active.Done()
	default:
	}
}
func (r *Run) beginClose() {
	r.once.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.cancel()
		r.mu.Unlock()
		go func() {
			r.active.Wait()
			close(r.drained)
		}()
	})
}
func (r *Run) close() {
	r.beginClose()
	<-r.drained
}

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
	x := &Run{ID: base64.RawURLEncoding.EncodeToString(b), Owner: owner, Expires: time.Now().Add(r.ttl), ctx: ctx, cancel: cancel, slots: make(chan struct{}, r.streams), drained: make(chan struct{})}
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
func (r *Registry) CloseOwned(id, owner string) error {
	r.mu.Lock()
	x := r.runs[id]
	if x == nil || x.Owner != owner {
		r.mu.Unlock()
		return errors.New("not found")
	}
	x.beginClose()
	r.mu.Unlock()
	r.finishClose(id, x)
	return nil
}
func (r *Registry) Close(id string) {
	r.mu.Lock()
	x := r.runs[id]
	if x != nil {
		x.beginClose()
	}
	r.mu.Unlock()
	if x != nil {
		r.finishClose(id, x)
	}
}
func (r *Registry) CloseAll() {
	r.mu.Lock()
	runs := make(map[string]*Run, len(r.runs))
	for id, x := range r.runs {
		runs[id] = x
		x.beginClose()
	}
	r.mu.Unlock()
	for id, x := range runs {
		r.finishClose(id, x)
	}
}
func (r *Registry) finishClose(id string, x *Run) {
	<-x.drained
	r.mu.Lock()
	if r.runs[id] == x {
		delete(r.runs, id)
	}
	r.mu.Unlock()
}
func (r *Registry) cleanupLocked() {
	for k, x := range r.runs {
		if x.ctx.Err() != nil || time.Now().After(x.Expires) {
			x.beginClose()
			select {
			case <-x.drained:
				delete(r.runs, k)
			default:
			}
		}
	}
}
