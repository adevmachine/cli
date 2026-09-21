package dns

import (
	"errors"
	"strings"
	"testing"
)

func TestSupportedTypeNormalisesToUpperCase(t *testing.T) {
	got, err := SupportedType("cname")
	if err != nil {
		t.Fatalf("SupportedType returned %v", err)
	}
	if got != "CNAME" {
		t.Fatalf("got %q, want %q", got, "CNAME")
	}
}

func TestSupportedTypeRefusesMXByName(t *testing.T) {
	_, err := SupportedType("MX")
	if !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("got %v, want ErrUnsupportedType", err)
	}
	for _, want := range []string{"MX", "A", "AAAA", "CNAME", "TXT"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error does not mention %q: %v", want, err)
		}
	}
}

func TestErrorForKindMapsEveryKindAProviderCanReport(t *testing.T) {
	cases := map[string]error{
		"zone_not_found":  ErrZoneNotFound,
		"unauthenticated": ErrUnauthenticated,
		"forbidden":       ErrForbidden,
		"invalid_record":  ErrInvalidRecord,
		"rate_limited":    ErrRateLimited,
		"ambiguous":       ErrAmbiguous,
	}
	for kind, want := range cases {
		if got := ErrorForKind(kind, "what the API said"); !errors.Is(got, want) {
			t.Errorf("kind %q gave %v, want %v", kind, got, want)
		}
	}
}

func TestErrorForKindKeepsTheProvidersOwnMessage(t *testing.T) {
	err := ErrorForKind("unauthenticated", "Authentication error")
	if !strings.Contains(err.Error(), "Authentication error") {
		t.Fatalf("the provider's own words were dropped: %v", err)
	}
}

func TestErrorForKindDoesNotSwallowAKindItDoesNotKnow(t *testing.T) {
	err := ErrorForKind("teapot", "something new")
	if err == nil {
		t.Fatal("an unknown kind became success")
	}
	if !strings.Contains(err.Error(), "teapot") || !strings.Contains(err.Error(), "something new") {
		t.Fatalf("an unknown kind must still reach the user: %v", err)
	}
}
