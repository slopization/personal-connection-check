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
