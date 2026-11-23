package main

import "testing"

func TestLoginEncryptionMatchesBrowserPacking(t *testing.T) {
	const script = `function _E() { return "\x30\x33"; }
function _M() { return "010000000001"; }
function _DK() { return "\x70\x3a"; }`

	key, err := parseLoginKey(script)
	if err != nil {
		t.Fatal(err)
	}
	const want = "000a7ee59d5e 00f96eca36c5 007820fe554f 0008b699f0da 000ded21"
	if got := key.encrypt("中 a"); got != want {
		t.Fatalf("ciphertext = %q, want %q", got, want)
	}
}
