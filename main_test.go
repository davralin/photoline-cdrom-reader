package main

import "testing"

func TestValidateListenAddressLoopbackByDefault(t *testing.T) {
	for _, address := range []string{"127.0.0.1:0", "[::1]:8080", "localhost:8080"} {
		if err := validateListenAddress(address, false); err != nil {
			t.Errorf("validateListenAddress(%q) = %v, want nil", address, err)
		}
	}
}

func TestValidateListenAddressRequiresExplicitNonLoopbackOptIn(t *testing.T) {
	for _, address := range []string{"0.0.0.0:8080", ":8080", "192.0.2.1:8080"} {
		if err := validateListenAddress(address, false); err == nil {
			t.Errorf("validateListenAddress(%q) = nil, want rejection", address)
		}
		if err := validateListenAddress(address, true); err != nil {
			t.Errorf("explicit container bind %q rejected: %v", address, err)
		}
	}
}

func TestValidateListenAddressRejectsMalformedAddress(t *testing.T) {
	if err := validateListenAddress("not-an-address", true); err == nil {
		t.Fatal("malformed listen address unexpectedly accepted")
	}
}
