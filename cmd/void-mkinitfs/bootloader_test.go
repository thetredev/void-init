package main

import "testing"

func TestByUUIDSymlink(t *testing.T) {
	got := byUUIDSymlink("/dev/nbd0p3", "1234-5678")
	want := "ln -sf ../../nbd0p3 /dev/disk/by-uuid/1234-5678"

	if got != want {
		t.Errorf("byUUIDSymlink() = %q, want %q", got, want)
	}
}
