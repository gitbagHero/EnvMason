package version

import "testing"

func FuzzVersionParsersNeverPanic(f *testing.F) {
	for _, seed := range []string{"1.2.3", "v22.17.0", "1.8.0_361", "25.0.3+9-LTS", "26-ea+5", "", "\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		for _, value := range []Value{ParseSemVer(raw), ParseJava(raw)} {
			relation := Compare(value, value)
			if value.Comparable && relation != RelationEqual {
				t.Fatalf("comparable value is not reflexive: %#v = %s", value, relation)
			}
			if !value.Comparable && relation != RelationUnknown {
				t.Fatalf("incomparable value returned %s", relation)
			}
		}
	})
}
