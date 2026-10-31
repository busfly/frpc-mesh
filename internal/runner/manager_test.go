package runner

import "testing"

func TestStripANSI(t *testing.T) {
	cases := map[string]string{
		"\x1b[1;34m2026-10-31 [I] start frpc\x1b[0m": "2026-10-31 [I] start frpc",
		"plain line without color":                   "plain line without color",
		"[0m [1;34m2026-10-31 login success\x1b[0m":  "[0m [1;34m2026-10-31 login success",
	}
	for in, want := range cases {
		if got := stripANSI(in); got != want {
			t.Errorf("stripANSI(%q) = %q, want %q", in, got, want)
		}
	}
}
