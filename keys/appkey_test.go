package keys

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// An app key of any length but 64 used to reach the signing code, where a
// wrong-sized ed25519 private key panics rather than failing. These lengths are
// the ones that were measured panicking: 4 and 31 on a slice bound inside
// go.sia.tech/core, 32 and 63 inside crypto/ed25519.
func TestOnlyAnEd25519SizedAppKeyIsAccepted(t *testing.T) {
	for _, n := range []int{1, 4, 16, 31, 32, 33, 63, 65, 128} {
		key := AppKey(make([]byte, n))
		err := key.Check()
		if err == nil {
			t.Errorf("a %d-byte app key was accepted; only %d bytes is a private key",
				n, ed25519.PrivateKeySize)
			continue
		}
		if !WrongAppKeyLength(err) {
			t.Errorf("a %d-byte app key gave %v, which is not classified as a length failure", n, err)
		}
		// The two failures ask the user for different things, so a damaged key
		// must never be reported as an absent one.
		if MissingAppKey(err) {
			t.Errorf("a %d-byte app key was classified as a missing key", n)
		}
		if !strings.Contains(err.Error(), "64") {
			t.Errorf("a %d-byte app key gave %q, which does not say what the length should be", n, err)
		}
	}

	if err := AppKey(make([]byte, ed25519.PrivateKeySize)).Check(); err != nil {
		t.Errorf("a %d-byte app key was refused: %v", ed25519.PrivateKeySize, err)
	}
}

// An empty key is the absent case, not the damaged one: it sends the reader to
// the approval instructions rather than telling them to re-copy what they have.
func TestAnEmptyAppKeyIsMissingRatherThanMalformed(t *testing.T) {
	err := AppKey(nil).Check()
	if !MissingAppKey(err) {
		t.Fatalf("an empty app key gave %v, which is not the absent-key case", err)
	}
	if WrongAppKeyLength(err) {
		t.Error("an empty app key was reported as a damaged one")
	}
}

func TestAppKeyFromEnvRefusesWhatWouldPanicLater(t *testing.T) {
	valid := strings.Repeat("ab", ed25519.PrivateKeySize)

	for _, c := range []struct {
		name    string
		set     string
		unset   bool
		missing bool // expect the absent case rather than the damaged one
		wantErr bool
	}{
		{name: "unset", unset: true, missing: true, wantErr: true},
		{name: "empty", set: "", missing: true, wantErr: true},
		{name: "whitespace only", set: "   \t\n ", missing: true, wantErr: true},
		{name: "truncated to 32 bytes", set: strings.Repeat("ab", 32), wantErr: true},
		{name: "one byte short", set: strings.Repeat("ab", 63), wantErr: true},
		{name: "one byte long", set: strings.Repeat("ab", 65), wantErr: true},
		{name: "one hex digit lost", set: valid[:len(valid)-1], wantErr: true},
		{name: "not hex at all", set: strings.Repeat("zz", 64), wantErr: true},
		{name: "surrounded by whitespace", set: "  " + valid + "\n"},
		{name: "the whole key", set: valid},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.unset {
				// t.Setenv records the original and restores it at cleanup, so
				// removing the variable afterwards is safe and reversible.
				t.Setenv(AppKeyEnv, "")
				if err := os.Unsetenv(AppKeyEnv); err != nil {
					t.Fatalf("unset %s: %v", AppKeyEnv, err)
				}
			} else {
				t.Setenv(AppKeyEnv, c.set)
			}

			key, err := AppKeyFromEnv()
			if !c.wantErr {
				if err != nil {
					t.Fatalf("a valid app key was refused: %v", err)
				}
				if len(key) != ed25519.PrivateKeySize {
					t.Fatalf("returned %d bytes, not a private key", len(key))
				}
				return
			}
			if err == nil {
				t.Fatalf("accepted %q, which cannot sign", c.set)
			}
			if c.missing != MissingAppKey(err) {
				t.Fatalf("MissingAppKey=%v for %q, wanted %v (err: %v)",
					MissingAppKey(err), c.set, c.missing, err)
			}
			if key != nil {
				t.Errorf("returned a key alongside the error: %d bytes", len(key))
			}
		})
	}
}

// Bad hex is a different failure from the right hex at the wrong length, and
// saying "wrong length" for a value that never decoded would send the reader
// looking for a truncation that is not there.
func TestUndecodableHexIsNotReportedAsALengthProblem(t *testing.T) {
	t.Setenv(AppKeyEnv, strings.Repeat("gg", 64))
	_, err := AppKeyFromEnv()
	if err == nil {
		t.Fatal("accepted a value that is not hex")
	}
	if WrongAppKeyLength(err) {
		t.Errorf("undecodable hex was reported as a length failure: %v", err)
	}
	if !strings.Contains(err.Error(), AppKeyEnv) {
		t.Errorf("the error does not name %s: %v", AppKeyEnv, err)
	}
}

// A real key from the approval path is what Check has to accept, so it is
// generated the way ed25519 generates one rather than assumed to be 64 zeros.
func TestAGeneratedPrivateKeyPasses(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	t.Setenv(AppKeyEnv, hex.EncodeToString(priv))
	key, err := AppKeyFromEnv()
	if err != nil {
		t.Fatalf("a generated ed25519 private key was refused: %v", err)
	}
	if err := key.Check(); err != nil {
		t.Fatalf("the key it returned does not pass its own check: %v", err)
	}
	// The value survives the round trip through hex and the environment.
	if string(key) != string(priv) {
		t.Error("the key read back differs from the one set")
	}
}
