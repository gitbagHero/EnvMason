package version

import "testing"

func TestSemVerNormalizationAndComparison(t *testing.T) {
	t.Parallel()
	tests := []struct {
		left, right string
		want        Relation
	}{
		{"v22.17.0", "22.17.0", RelationEqual},
		{"1.0.0-alpha", "1.0.0-alpha.1", RelationLess},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", RelationLess},
		{"1.0.0-beta.2", "1.0.0-beta.11", RelationLess},
		{"1.0.0-rc.1", "1.0.0", RelationLess},
		{"1.0.0+build.1", "1.0.0+build.2", RelationEqual},
		{"999999999999999999999.0.0", "2.0.0", RelationGreater},
	}
	for _, test := range tests {
		left, right := ParseSemVer(test.left), ParseSemVer(test.right)
		if !left.Comparable || !right.Comparable {
			t.Fatalf("valid SemVer rejected: %q / %q", test.left, test.right)
		}
		if got := Compare(left, right); got != test.want {
			t.Errorf("Compare(%q, %q) = %s, want %s", test.left, test.right, got, test.want)
		}
		if got := Compare(right, left); got != invert(test.want) {
			t.Errorf("reverse Compare(%q, %q) = %s", test.right, test.left, got)
		}
	}
	if got := ParseSemVer("v22.17.0").Normalized; got != "22.17.0" {
		t.Fatalf("normalized Node version = %q", got)
	}
}

func TestSemVerRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "1", "1.2", "01.2.3", "1.02.3", "1.2.03", "1.2.3-01", "1.2.3-", "1.2.3+", " 1.2.3", "1.2.3\n", "V1.2.3", "1.2.3/evil"} {
		if value := ParseSemVer(raw); value.Comparable || Compare(value, value) != RelationUnknown {
			t.Errorf("invalid SemVer %q became comparable: %#v", raw, value)
		}
	}
}

func TestJavaNormalizationAndComparison(t *testing.T) {
	t.Parallel()
	tests := []struct {
		left, right string
		want        Relation
	}{
		{"1.8.0_361", "8u361", RelationEqual},
		{"8u361", "8u441-b07", RelationLess},
		{"17.0.14", "21.0.7", RelationLess},
		{"21.0.7+6-LTS", "21.0.7+9-tem", RelationEqual},
		{"21.0.7-amzn", "21.0.7-microsoft", RelationEqual},
		{"26-ea+5", "26-ea+6", RelationLess},
		{"26-ea+99", "26", RelationLess},
		{"25.0.3", "25.0.3.0", RelationEqual},
	}
	for _, test := range tests {
		left, right := ParseJava(test.left), ParseJava(test.right)
		if !left.Comparable || !right.Comparable {
			t.Fatalf("valid Java version rejected: %q=%#v / %q=%#v", test.left, left, test.right, right)
		}
		if got := Compare(left, right); got != test.want {
			t.Errorf("Compare(%q, %q) = %s, want %s", test.left, test.right, got, test.want)
		}
		if got := Compare(right, left); got != invert(test.want) {
			t.Errorf("reverse Compare(%q, %q) = %s", test.right, test.left, got)
		}
	}
	for raw, want := range map[string]string{
		"1.8.0_361":     "8.0.361",
		"8u441-b07":     "8.0.441+7",
		"25.0.3+09-LTS": "25.0.3+9-lts",
		"26-ea+05":      "26-ea+5",
	} {
		if got := ParseJava(raw).Normalized; got != want {
			t.Errorf("ParseJava(%q).Normalized = %q, want %q", raw, got, want)
		}
	}
}

func TestJavaRejectsAmbiguousValues(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "latest", "21..1", "21-beta", "21+build", "21+1+2", "1.8.0_", "21-unknownvendor", "21/1", " 21.0.1", "21.0.1\n"} {
		if value := ParseJava(raw); value.Comparable || Compare(value, value) != RelationUnknown {
			t.Errorf("ambiguous Java version %q became comparable: %#v", raw, value)
		}
	}
}

func TestComparisonLaws(t *testing.T) {
	t.Parallel()
	for name, values := range map[string][]Value{
		"semver": parseSemVers([]string{"1.0.0-alpha", "1.0.0", "1.2.0", "2.0.0", "10.0.0"}),
		"java":   parseJavaVersions([]string{"8u361", "11.0.22", "17.0.14", "21.0.7", "25.0.3", "26-ea+5", "26"}),
	} {
		t.Run(name, func(t *testing.T) {
			for i := range values {
				for j := range values {
					if got, reverse := Compare(values[i], values[j]), Compare(values[j], values[i]); reverse != invert(got) {
						t.Fatalf("antisymmetry failed at %d/%d: %s/%s", i, j, got, reverse)
					}
					for k := range values {
						if Compare(values[i], values[j]) == RelationLess && Compare(values[j], values[k]) == RelationLess && Compare(values[i], values[k]) != RelationLess {
							t.Fatalf("transitivity failed at %d/%d/%d", i, j, k)
						}
					}
				}
			}
		})
	}
	if got := Compare(ParseSemVer("1.0.0"), ParseJava("1.0.0")); got != RelationUnknown {
		t.Fatalf("cross-scheme comparison = %s", got)
	}
}

func parseSemVers(values []string) []Value {
	result := make([]Value, len(values))
	for index, value := range values {
		result[index] = ParseSemVer(value)
	}
	return result
}

func parseJavaVersions(values []string) []Value {
	result := make([]Value, len(values))
	for index, value := range values {
		result[index] = ParseJava(value)
	}
	return result
}
