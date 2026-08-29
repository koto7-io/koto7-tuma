package config

import "testing"

func TestDemoModeEnabled(t *testing.T) {
	t.Setenv("TUMA_DEMO_MODE", "")
	tests := []struct {
		env  string
		url  string
		want bool
	}{
		{"true", "http://localhost", true},
		{"false", "https://tuma-demo.koto7.dev", false},
		{"", "https://tuma-demo.koto7.dev", true},
		{"", "http://localhost", false},
	}
	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			t.Setenv("TUMA_DEMO_MODE", tc.env)
			if got := demoModeEnabled(tc.url); got != tc.want {
				t.Fatalf("demoModeEnabled(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}
