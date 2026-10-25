package pull

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.2.3", "v1.2.3", 0},
		{"v1.2.3", "v1.2.4", -1},
		{"v1.10.0", "v1.9.9", 1},
		{"v2.0.0", "v1.99.99", 1},
		{"v5.0.0-rc.1", "v5.0.0", -1},
		{"v5.0.0", "v5.0.0-rc.1", 1},
		{"v5.0.0-rc.2", "v5.0.0-rc.10", -1},
		{"v5.0.0-alpha", "v5.0.0-beta", -1},
		{"v5.0.0-1", "v5.0.0-alpha", -1},
		{"v5.0.0-rc", "v5.0.0-rc.1", -1},
		{"v1.2.3+incompatible", "v1.2.3", 0},
	}
	for _, c := range cases {
		a, err := ParseVersion(c.a)
		if err != nil {
			t.Fatal(err)
		}
		b, err := ParseVersion(c.b)
		if err != nil {
			t.Fatal(err)
		}
		if got := CompareVersions(a, b); got != c.want {
			t.Errorf("CompareVersions(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
		if back := CompareVersions(b, a); back != -c.want {
			t.Errorf("CompareVersions(%s, %s) = %d, want %d", c.b, c.a, back, -c.want)
		}
	}
}

func TestParseVersionPrerelease(t *testing.T) {
	v, err := ParseVersion("v5.0.0-rc.1+build.5")
	if err != nil || !v.Prerelease() || v.Pre != "rc.1" || v.Major != 5 {
		t.Fatalf("version = %+v %v", v, err)
	}
	stable, _ := ParseVersion("v5.3.1")
	if stable.Prerelease() {
		t.Fatal("a release is not a prerelease")
	}
	if _, err := ParseVersion("5.3.1"); err == nil {
		t.Fatal("a version without the v prefix was accepted")
	}
}
