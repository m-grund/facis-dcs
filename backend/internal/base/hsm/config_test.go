package hsm

import "testing"

func TestVersionedLabel(t *testing.T) {
	cases := []struct {
		base    string
		version int
		want    string
	}{
		{"dcs-contract-pades", 0, "dcs-contract-pades"},
		{"dcs-contract-pades", 1, "dcs-contract-pades"},
		{"dcs-contract-pades", 2, "dcs-contract-pades-v2"},
		{"dcs-contract-pades", 5, "dcs-contract-pades-v5"},
	}
	for _, c := range cases {
		if got := VersionedLabel(c.base, c.version); got != c.want {
			t.Errorf("VersionedLabel(%q, %d) = %q, want %q", c.base, c.version, got, c.want)
		}
	}
}
