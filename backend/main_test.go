package main

import "testing"

func TestListenAddrUsesAddrWhenSet(t *testing.T) {
	t.Setenv("ADDR", ":9090")
	t.Setenv("PORT", "3000")

	if got := listenAddr(); got != ":9090" {
		t.Fatalf("listenAddr() = %q, want %q", got, ":9090")
	}
}

func TestListenAddrUsesVercelPort(t *testing.T) {
	t.Setenv("ADDR", "")
	t.Setenv("PORT", "3000")

	if got := listenAddr(); got != ":3000" {
		t.Fatalf("listenAddr() = %q, want %q", got, ":3000")
	}
}

func TestListenAddrDefaultsToLocalPort(t *testing.T) {
	t.Setenv("ADDR", "")
	t.Setenv("PORT", "")

	if got := listenAddr(); got != ":8080" {
		t.Fatalf("listenAddr() = %q, want %q", got, ":8080")
	}
}
