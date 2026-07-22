package meteora

import (
	"strings"
	"testing"
)

func TestMomentumRejectTightenedDumpGates(t *testing.T) {
	cases := []struct {
		name string
		m    Momentum
		want string
	}{
		{"m5 dump", Momentum{M5: -3.1}, "5m -3.1% <= -3%"},
		{"h1 dump", Momentum{M5: 0, H1: -7.1}, "1h -7.1% <= -7%"},
		{"h6 downtrend", Momentum{M5: 0, H1: 0, H6: -10.1}, "6h -10.1% <= -10%"},
		{"h24 downtrend", Momentum{M5: 0, H1: 0, H6: 0, H24: -20.1}, "24h -20.1% <= -20%"},
		{"passes", Momentum{M5: -2.9, H1: -6.9, H6: -9.9, H24: -19.9}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MomentumReject(tc.m)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("MomentumReject() = %q, want pass", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("MomentumReject() = %q, want contains %q", got, tc.want)
			}
		})
	}
}
