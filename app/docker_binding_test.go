package main

import "testing"

func TestManagedPortBindingUsesLoopback(t *testing.T) {
	got := managedPortBinding(8081, 3000)
	want := "127.0.0.1:8081:3000"

	if got != want {
		t.Fatalf(
			"expected %q, got %q",
			want,
			got,
		)
	}
}
