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
