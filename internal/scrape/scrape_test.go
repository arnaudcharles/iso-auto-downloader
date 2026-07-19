package scrape

import "testing"

func TestLooksLikeVersion(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"24.04", true},
		{"26.04.1", true},
		{"8.10", true},
		{"20260714", true},
		{"25H2", true},
		{"unknown", false},
		{"", false},
		{"latest", false},
	}
	for _, c := range cases {
		if got := LooksLikeVersion(c.s); got != c.want {
			t.Errorf("LooksLikeVersion(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

// TestCompareVersionsWindowsFeatureUpdate is a regression test for a real
// report: the disk scan picking a stale, manually-downloaded Windows 11
// 23H2 ISO over a genuinely present, complete, newer 25H2 one this app had
// downloaded itself, because "25H2" and "23H2" each parse as a single
// non-numeric segment — strconv.Atoi fails on both, so a naive comparison
// silently treated them as equal (0), and whichever file the directory
// walk happened to see first "won" and was never displaced. A user ended
// up watching the app re-download an already-current 8GB ISO because the
// found version compared as neither older nor newer.
func TestCompareVersionsWindowsFeatureUpdate(t *testing.T) {
	if got := CompareVersions("25H2", "23H2"); got <= 0 {
		t.Errorf(`CompareVersions("25H2", "23H2") = %d, want > 0`, got)
	}
	if got := CompareVersions("23H2", "25H2"); got >= 0 {
		t.Errorf(`CompareVersions("23H2", "25H2") = %d, want < 0`, got)
	}
	if got := CompareVersions("25H2", "25H2"); got != 0 {
		t.Errorf(`CompareVersions("25H2", "25H2") = %d, want 0`, got)
	}
	if got := CompareVersions("25H2", "25H1"); got <= 0 {
		t.Errorf(`CompareVersions("25H2", "25H1") = %d, want > 0`, got)
	}
}

func TestCompareVersionsNumericDotted(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"24.04", "23.10", 1},
		{"24.04", "24.04", 0},
		{"24.04", "24.04.1", -1},
		{"24.04.1", "24.04", 1},
		{"8.10", "8.9", 1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
