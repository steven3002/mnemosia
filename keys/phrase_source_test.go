package keys

import (
	"os"
	"strings"
	"testing"
)

// A terminal is refused rather than read, so a first run gets the instructions
// instead of a silent block it cannot interrupt.
//
// The device stands in for a terminal because it is a character device and can
// be opened anywhere; what ReadPhrase tests for is exactly that.
func TestAPhraseIsNotReadFromACharacterDevice(t *testing.T) {
	t.Setenv(PhraseEnv, "")

	tty, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer tty.Close()

	phrase, err := ReadPhrase(tty)
	if err == nil {
		t.Fatalf("a character device was read for a phrase, and returned %q", phrase)
	}
	if err != ErrNoPhrase {
		t.Errorf("gave %v, not the instructions a first run needs", err)
	}
}

// The supported path is unaffected: a pipe is how a phrase is actually handed
// over, and refusing terminals must not touch it.
func TestAPhraseStillArrivesOnAPipe(t *testing.T) {
	t.Setenv(PhraseEnv, "")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()

	const want = "abandon abandon abandon abandon abandon about"
	go func() {
		defer w.Close()
		w.WriteString("  " + want + "  \n")
	}()

	got, err := ReadPhrase(r)
	if err != nil {
		t.Fatalf("a phrase on a pipe was refused: %v", err)
	}
	if got != want {
		t.Errorf("read %q, wanted %q", got, want)
	}
}

// The environment still wins over any reader, terminal or not, so setting it
// remains the way to avoid the question entirely.
func TestTheEnvironmentIsPreferredEvenOverATerminal(t *testing.T) {
	t.Setenv(PhraseEnv, " from the environment ")

	tty, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer tty.Close()

	got, err := ReadPhrase(tty)
	if err != nil {
		t.Fatalf("read phrase: %v", err)
	}
	if got != "from the environment" {
		t.Errorf("read %q", got)
	}
}

// A reader that is not a file cannot be a terminal, so the in-process callers
// that pass a plain reader keep working.
func TestAPlainReaderIsNotMistakenForATerminal(t *testing.T) {
	t.Setenv(PhraseEnv, "")

	got, err := ReadPhrase(strings.NewReader("a phrase from memory\n"))
	if err != nil {
		t.Fatalf("read phrase: %v", err)
	}
	if got != "a phrase from memory" {
		t.Errorf("read %q", got)
	}
}
