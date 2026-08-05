package local_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/steven3002/mnemosia/local"
	"github.com/steven3002/mnemosia/record"
)

func TestBodyRoundTrip(t *testing.T) {
	store, err := local.Open(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	id, _ := record.NewID()
	body := []byte(`{"statement":"a fact worth keeping"}`)
	if err := store.PutBody(id, record.KindMemory, body); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := store.GetBody(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("body came back as %q", got)
	}
}

func TestVectorRoundTrip(t *testing.T) {
	store, err := local.Open(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	id, _ := record.NewID()
	if err := store.PutBody(id, record.KindMemory, []byte("{}")); err != nil {
		t.Fatalf("put body: %v", err)
	}
	want := []float32{0.5, -0.25, 0.125}
	if err := store.PutVector(id, "test-model", want); err != nil {
		t.Fatalf("put vector: %v", err)
	}

	stored, err := store.Vectors()
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	if len(stored) != 1 || stored[0].ID != id || stored[0].Model != "test-model" {
		t.Fatalf("stored vectors are %+v", stored)
	}
	for n, v := range want {
		if stored[0].Values[n] != v {
			t.Fatalf("dimension %d came back as %v, want %v", n, stored[0].Values[n], v)
		}
	}
}

// Every protocol client launches its own process against one vault, so
// concurrent writers are the normal case rather than an edge one.
func TestConcurrentProcessesShareOneDatabase(t *testing.T) {
	if os.Getenv(writerEnv) != "" {
		runWriter(t)
		return
	}
	const (
		writers = 3
		rows    = 200
	)
	path := filepath.Join(t.TempDir(), "vault.db")

	// The database has to exist before the writers race for it: creating it is
	// the one step that needs a lock no other connection holds.
	seed, err := local.Open(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	seed.Close()

	var wg sync.WaitGroup
	failures := make([]error, writers)
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run", "^TestConcurrentProcessesShareOneDatabase$")
			cmd.Env = append(os.Environ(),
				writerEnv+"="+strconv.Itoa(w),
				pathEnv+"="+path,
				rowsEnv+"="+strconv.Itoa(rows))
			if out, err := cmd.CombinedOutput(); err != nil {
				failures[w] = fmt.Errorf("writer %d: %w\n%s", w, err, out)
			}
		}()
	}
	wg.Wait()
	for _, err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}

	store, err := local.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer store.Close()

	count, err := store.CountBodies()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if want := writers * rows; count != want {
		t.Fatalf("%d records survived %d concurrent writers, want %d", count, writers, want)
	}
}

// runWriter is the child half of the concurrency test: one process writing into
// a database two others are writing to at the same time.
func runWriter(t *testing.T) {
	path := os.Getenv(pathEnv)
	rows, err := strconv.Atoi(os.Getenv(rowsEnv))
	if err != nil {
		t.Fatalf("bad %s: %v", rowsEnv, err)
	}
	store, err := local.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	for range rows {
		id, err := record.NewID()
		if err != nil {
			t.Fatalf("new id: %v", err)
		}
		if err := store.PutBody(id, record.KindMemory, []byte("{}")); err != nil {
			t.Fatalf("put: %v", err)
		}
	}
}

const (
	writerEnv = "MNEMOSIA_TEST_WRITER"
	pathEnv   = "MNEMOSIA_TEST_DB"
	rowsEnv   = "MNEMOSIA_TEST_ROWS"
)
