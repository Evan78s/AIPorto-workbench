package util

import (
	"testing"
	"time"
)

func TestVerifyTOTP_RFC6238Vector(t *testing.T) {
	// RFC 6238 附录 B 的标准向量：secret = ASCII "12345678901234567890"。
	secret := []byte("12345678901234567890")
	cases := []struct {
		at   int64
		code string
	}{
		// RFC 6238 给出的是 8 位码，这里取其标准 6 位 TOTP（后 6 位）。
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
	}
	for _, tc := range cases {
		got, ok := VerifyTOTP(secret, tc.code, time.Unix(tc.at, 0), 0)
		if !ok {
			t.Fatalf("at=%d expected %s to verify", tc.at, tc.code)
		}
		want := uint64(tc.at / 30)
		if got != want {
			t.Fatalf("at=%d step=%d want %d", tc.at, got, want)
		}
	}
}

func TestVerifyTOTP_RejectsWrongAndMalformed(t *testing.T) {
	secret := []byte("12345678901234567890")
	if _, ok := VerifyTOTP(secret, "000000", time.Unix(59, 0), 1); ok {
		t.Fatal("wrong code must not verify")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if _, ok := VerifyTOTP(secret, bad, time.Unix(59, 0), 1); ok {
			t.Fatalf("malformed code %q must not verify", bad)
		}
	}
}

func TestDecodeTOTPSecret(t *testing.T) {
	in := "gezdgnbvgy3tqojqgezdgnbvgy3tqojq"
	b, err := DecodeTOTPSecret(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "12345678901234567890" {
		t.Fatalf("decoded %q", string(b))
	}
	if _, err := DecodeTOTPSecret("!!!"); err == nil {
		t.Fatal("invalid base32 must fail")
	}
	if _, err := DecodeTOTPSecret("MZXW6"); err == nil {
		t.Fatal("too-short secret must fail")
	}
}
