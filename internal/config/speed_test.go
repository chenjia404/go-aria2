package config

import "testing"

func TestParseSpeedBytes(t *testing.T) {
	t.Parallel()

	cases := map[string]int64{
		"50K":    50 * 1024,
		"300K":   300 * 1024,
		"1M":     1024 * 1024,
		"1024":   1024,
		"0":      0,
	}
	for input, want := range cases {
		got, err := parseSpeedBytes(input)
		if err != nil {
			t.Fatalf("%q: %v", input, err)
		}
		if got != want {
			t.Fatalf("%q: got %d want %d", input, got, want)
		}
	}
}
