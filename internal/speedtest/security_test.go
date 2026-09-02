package speedtest

import (
	"context"
	"testing"
	"time"
)

func TestRunStreamLimitExpiryAndDeterministicRelease(t *testing.T) {
	r := New(2, 1, 2, 80*time.Millisecond)
	run, err := r.Create("session-a")
	if err != nil {
		t.Fatal(err)
	}
	if !run.Acquire(context.Background()) || !run.Acquire(context.Background()) {
		t.Fatal("expected two stream slots")
	}
	if run.Acquire(context.Background()) {
		t.Fatal("per-run stream limit bypassed")
	}
	run.Release()
	run.Release()
	run.Release() // exactly-once release must not overfill
	if !run.Acquire(context.Background()) || !run.Acquire(context.Background()) {
		t.Fatal("release did not restore exact capacity")
	}
	time.Sleep(120 * time.Millisecond)
	if err := run.Context().Err(); err == nil {
		t.Fatal("run deadline did not cancel active streams")
	}
	if _, err := r.Get(run.ID, "session-a"); err == nil {
		t.Fatal("expired run remained usable")
	}
}
