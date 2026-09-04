package ghostty

import (
	"slices"
	"testing"
)

func TestCleanEnvStripsZmxSession(t *testing.T) {
	in := []string{"PATH=/usr/bin", "ZMX_SESSION=myproject", "HOME=/home/x"}
	got := cleanEnv(in)

	if slices.ContainsFunc(got, func(kv string) bool { return kv == "ZMX_SESSION=myproject" }) {
		t.Errorf("cleanEnv(%v) = %v, want ZMX_SESSION stripped", in, got)
	}
	if !slices.Contains(got, "PATH=/usr/bin") || !slices.Contains(got, "HOME=/home/x") {
		t.Errorf("cleanEnv(%v) = %v, want other vars untouched", in, got)
	}
}

func TestCleanEnvNoZmxSession(t *testing.T) {
	in := []string{"PATH=/usr/bin", "HOME=/home/x"}
	got := cleanEnv(in)

	if len(got) != len(in) {
		t.Errorf("cleanEnv(%v) = %v, want unchanged when ZMX_SESSION absent", in, got)
	}
}

func TestCleanEnvDoesNotMatchPrefix(t *testing.T) {
	// A var that merely starts with the same characters must survive --
	// only an exact "ZMX_SESSION=" key should be stripped.
	in := []string{"ZMX_SESSION_ID=unrelated"}
	got := cleanEnv(in)

	if !slices.Contains(got, "ZMX_SESSION_ID=unrelated") {
		t.Errorf("cleanEnv(%v) = %v, want ZMX_SESSION_ID left untouched", in, got)
	}
}
