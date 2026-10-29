package restorer

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Kenshoo-ops/kenshoo/internal/storage"
)

func TestPgDumpFetchDetectsOversizedDownload(t *testing.T) {
	store := storage.NewMemStore()
	store.Put("app/a.dump", []byte("12345"), time.Now())
	store.SetReportedSize("app/a.dump", 2) // smaller than the 5 bytes actually delivered

	a, err := pgDump{}.Fetch(context.Background(), store, "app/")
	if err == nil {
		a.Remove()
		t.Fatal("expected an error when more bytes arrive than the store reported")
	}
}

func TestArtifactRemoveDeletesFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "kenshoo-*.dump")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	a := Artifact{Path: f.Name()}
	if err := a.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(f.Name()); !os.IsNotExist(err) {
		t.Errorf("file still exists after Remove (stat error: %v)", err)
	}
}
