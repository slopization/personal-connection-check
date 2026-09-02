package speedtest

import (
	"testing"
	"time"
)

func TestRunRegistryLimitsAndOwnership(t *testing.T) {
	r := New(1, 1, time.Second)
	a, e := r.Create("a")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Create("a"); e == nil {
		t.Fatal("limit bypass")
	}
	if _, e = r.Get(a.ID, "b"); e == nil {
		t.Fatal("ownership bypass")
	}
	r.Close(a.ID)
	if _, e = r.Create("b"); e != nil {
		t.Fatal(e)
	}
}

func TestExpiryKeepsCapacityUntilActiveStreamRelease(t *testing.T) {
	r := New(1, 1, 1, 20*time.Millisecond)
	run, err := r.Create("owner")
	if err != nil {
		t.Fatal(err)
	}
	if !run.Acquire(t.Context()) {
		t.Fatal("failed to acquire test stream")
	}
	select {
	case <-run.Context().Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("run did not expire")
	}
	if _, err := r.Create("next"); err == nil {
		t.Fatal("expiry restored capacity before active stream release")
	}
	run.Release()
	deadline := time.Now().Add(100 * time.Millisecond)
	for {
		if _, err := r.Create("next"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expiry did not restore capacity after active stream release")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCloseOwnedWaitsForActiveStreamBeforeRestoringCapacity(t *testing.T) {
	r := New(1, 1, 1, time.Second)
	run, err := r.Create("owner")
	if err != nil {
		t.Fatal(err)
	}
	if !run.Acquire(t.Context()) {
		t.Fatal("failed to acquire test stream")
	}
	defer run.Release()
	closed := make(chan error, 1)
	go func() { closed <- r.CloseOwned(run.ID, "owner") }()

	select {
	case <-run.Context().Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("close did not publish cancellation")
	}
	select {
	case err := <-closed:
		t.Fatalf("close returned before active stream release: %v", err)
	default:
	}
	if run.Acquire(t.Context()) {
		t.Fatal("acquire succeeded after close began")
	}
	created := make(chan error, 1)
	go func() {
		_, err := r.Create("next")
		created <- err
	}()
	select {
	case err := <-created:
		if err == nil {
			t.Fatal("capacity restored before active stream release")
		}
	case <-time.After(20 * time.Millisecond):
		t.Fatal("registry blocked while active stream drained")
	}

	run.Release()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("close did not finish after active stream release")
	}
	if _, err := r.Create("next"); err != nil {
		t.Fatalf("capacity not restored after release: %v", err)
	}
}

func TestCloseOwnedRestoresCapacityOnlyForOwner(t *testing.T) {
	r := New(1, 1, time.Second)
	run, err := r.Create("owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.CloseOwned(run.ID, "other"); err == nil {
		t.Fatal("non-owner closed run")
	}
	if _, err := r.Create("other"); err == nil {
		t.Fatal("non-owner close restored capacity")
	}
	if err := r.CloseOwned(run.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Create("other"); err != nil {
		t.Fatalf("owner close did not restore capacity: %v", err)
	}
}
