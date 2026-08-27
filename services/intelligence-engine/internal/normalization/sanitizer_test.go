package normalization

import "testing"

func TestIsSensitiveAttribute(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"password", true},
		{"Authorization", true}, // case-insensitive
		{"AUTHORIZATION", true},
		{"api_key", true},
		{"secret", true},
		{"token", true},
		{"credential", true},
		{"x-auth-token", true},       // substring match: contains "token"
		{"user_password_hash", true}, // substring match: contains "password"
		{"client_secret", true},
		{"http.method", false},
		{"service.name", false},
		{"deployment.environment", false},
		{"", false},
	}

	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			if got := IsSensitiveAttribute(c.key); got != c.want {
				t.Errorf("IsSensitiveAttribute(%q) = %v, want %v", c.key, got, c.want)
			}
		})
	}
}

func TestSanitizeString_UnderLimit_ReturnsUnchanged(t *testing.T) {
	if got := SanitizeString("short", 100); got != "short" {
		t.Fatalf("expected unchanged string, got %q", got)
	}
}

func TestSanitizeString_ExactlyAtLimit_ReturnsUnchanged(t *testing.T) {
	s := "12345"
	if got := SanitizeString(s, len(s)); got != s {
		t.Fatalf("expected a string exactly at the limit to be returned unchanged, got %q", got)
	}
}

func TestSanitizeString_OverLimit_TruncatesWithSuffix(t *testing.T) {
	got := SanitizeString("abcdefghij", 5)
	want := "abcde...(truncated)"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSanitizeString_Empty(t *testing.T) {
	if got := SanitizeString("", 10); got != "" {
		t.Fatalf("expected an empty string to stay empty, got %q", got)
	}
}

func TestSanitizeString_ZeroLimit_TruncatesToJustTheSuffix(t *testing.T) {
	got := SanitizeString("anything", 0)
	want := "...(truncated)"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
