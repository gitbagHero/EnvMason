package java

import "testing"

func TestParsersRejectMalformedOrIncompleteData(t *testing.T) {
	t.Parallel()
	if _, err := parseJavaHomeXML(`<plist><array><dict>`, ""); err == nil {
		t.Fatal("malformed plist unexpectedly parsed")
	}
	if got := parseJavaRuntime("java.version = secret\n", ""); got.State != StateUnknown {
		t.Fatalf("incomplete runtime = %#v", got)
	}
	if got := parseMaven("not maven", ""); got.State != StateUnknown {
		t.Fatalf("invalid Maven output = %#v", got)
	}
	if got := parseGradle("not gradle", ""); got.State != StateUnknown {
		t.Fatalf("invalid Gradle output = %#v", got)
	}
}
