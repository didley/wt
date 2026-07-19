package main

import "testing"

const testLatest = "v0.3.2"

func TestNextVersion(t *testing.T) {
	cases := []struct {
		latest, bump, want string
	}{
		{testLatest, bumpPatch, "v0.3.3"},
		{testLatest, bumpMinor, "v0.4.0"},
		{testLatest, bumpMajor, "v1.0.0"},
		{"v0.0.0", bumpPatch, "v0.0.1"},
		{"v1.9.9", bumpMinor, "v1.10.0"},
		{testLatest, "v1.2.0", "v1.2.0"},
	}
	for _, tc := range cases {
		got, err := nextVersion(tc.latest, tc.bump)
		if err != nil {
			t.Errorf("nextVersion(%q, %q): %v", tc.latest, tc.bump, err)
			continue
		}
		if got != tc.want {
			t.Errorf("nextVersion(%q, %q) = %q, want %q", tc.latest, tc.bump, got, tc.want)
		}
	}
}

func TestNextVersionRejectsNonNewerExplicitVersion(t *testing.T) {
	for _, bump := range []string{"v0.1.0", testLatest} {
		if _, err := nextVersion(testLatest, bump); err == nil {
			t.Errorf("nextVersion(v0.3.2, %q): want an error, got nil", bump)
		}
	}
}

func TestNextVersionRejectsBadLatestTag(t *testing.T) {
	if _, err := nextVersion("not-a-version", bumpPatch); err == nil {
		t.Error("nextVersion with a malformed latest tag: want an error, got nil")
	}
}

func TestParseSemver(t *testing.T) {
	got, err := parseSemver("v1.2.3")
	if err != nil {
		t.Fatalf("parseSemver: %v", err)
	}
	if want := (semver{1, 2, 3}); got != want {
		t.Errorf("parseSemver(v1.2.3) = %+v, want %+v", got, want)
	}
	if _, err := parseSemver("1.2.3"); err == nil {
		t.Error("parseSemver without a leading v: want an error, got nil")
	}
}

func TestParseArgs(t *testing.T) {
	yes, bump := parseArgs([]string{"-y", bumpPatch})
	if !yes || bump != bumpPatch {
		t.Errorf("parseArgs([-y patch]) = (%v, %q), want (true, patch)", yes, bump)
	}

	yes, bump = parseArgs([]string{"v1.2.3"})
	if yes || bump != "v1.2.3" {
		t.Errorf("parseArgs([v1.2.3]) = (%v, %q), want (false, v1.2.3)", yes, bump)
	}
}
