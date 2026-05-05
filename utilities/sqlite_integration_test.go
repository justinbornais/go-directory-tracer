package utilities

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type generatedEntry struct {
	Name string `json:"n"`
	Type string `json:"t"`
	URL  string `json:"u,omitempty"`
}

func TestIndexFolderWithoutSQLitePreservesMetadataJSON(t *testing.T) {
	root := copyFixtureToTempDir(t)
	runFromDir(t, root, func() {
		IndexFolder(".", GenerateBoilerplateHTML("Fixture", "", ""), 0, nil, true, false, true, false, nil, nil)
	})

	rootEntries := readGeneratedEntries(t, filepath.Join(root, "data.json"))
	assertEntry(t, rootEntries, "albums", "d", "")
	assertEntry(t, rootEntries, "docs", "d", "")
	assertEntry(t, rootEntries, "song.mp3", "f", "https://fallback.example/root-song")
	assertEntry(t, rootEntries, "welcome.txt", "f", "")
	assertNoEntry(t, rootEntries, "index.html")

	albumEntries := readGeneratedEntries(t, filepath.Join(root, "albums", "data.json"))
	assertEntry(t, albumEntries, "live", "d", "")
	assertEntry(t, albumEntries, "track.mp3", "f", "https://fallback.example/album-track")

	assertFileExists(t, filepath.Join(root, "index.html"))
	assertFileExists(t, filepath.Join(root, "albums", "index.html"))
	assertFileExists(t, filepath.Join(root, "docs", "index.html"))
}

func TestIndexFolderWithSQLitePrefersDatabaseAndSyncsCatalog(t *testing.T) {
	root := copyFixtureToTempDir(t)
	catalog, err := OpenCatalog(root, "catalog.db")
	if err != nil {
		t.Fatalf("open sqlite catalog: %v", err)
	}
	defer catalog.Close()

	if err := catalog.UpsertFiles(".", []File{{Name: "song.mp3"}}); err != nil {
		t.Fatalf("seed root file in sqlite: %v", err)
	}
	if err := catalog.UpsertFiles("albums", []File{{Name: "track.mp3"}}); err != nil {
		t.Fatalf("seed album file in sqlite: %v", err)
	}

	_, err = catalog.db.Exec(`
INSERT INTO file_metadata (file_id, external_link, updated_at)
VALUES ((SELECT id FROM files WHERE rel_path = ?), ?, CURRENT_TIMESTAMP)
ON CONFLICT(file_id) DO UPDATE SET
  external_link = excluded.external_link,
  updated_at = CURRENT_TIMESTAMP
`, "song.mp3", "https://db.example/root-song")
	if err != nil {
		t.Fatalf("seed sqlite metadata: %v", err)
	}

	runFromDir(t, root, func() {
		IndexFolder(".", GenerateBoilerplateHTML("Fixture", "", ""), 0, nil, true, false, true, false, catalog, nil)
	})

	rootEntries := readGeneratedEntries(t, filepath.Join(root, "data.json"))
	assertEntry(t, rootEntries, "song.mp3", "f", "https://db.example/root-song")
	assertEntry(t, rootEntries, "welcome.txt", "f", "")
	assertNoEntry(t, rootEntries, "catalog.db")

	albumEntries := readGeneratedEntries(t, filepath.Join(root, "albums", "data.json"))
	assertEntry(t, albumEntries, "track.mp3", "f", "https://fallback.example/album-track")

	assertCatalogHasRow(t, catalog, `SELECT COUNT(1) FROM directories WHERE rel_path = ?`, "albums")
	assertCatalogHasRow(t, catalog, `SELECT COUNT(1) FROM files WHERE rel_path = ?`, "docs/guide.txt")
	assertCatalogHasRow(t, catalog, `SELECT COUNT(1) FROM files WHERE rel_path = ?`, "albums/live/encore.txt")
}

func copyFixtureToTempDir(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	fixtureRoot := filepath.Join("testdata", "site-fixture")

	err := filepath.Walk(fixtureRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(fixtureRoot, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		targetPath := filepath.Join(root, relPath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, data, 0644)
	})
	if err != nil {
		t.Fatalf("copy test fixture: %v", err)
	}

	return root
}

func runFromDir(t *testing.T, dir string, fn func()) {
	t.Helper()

	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	defer os.Chdir(previousDir)

	fn()
}

func readGeneratedEntries(t *testing.T, filePath string) []generatedEntry {
	t.Helper()

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read generated JSON %q: %v", filePath, err)
	}

	var entries []generatedEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("parse generated JSON %q: %v", filePath, err)
	}

	return entries
}

func assertEntry(t *testing.T, entries []generatedEntry, name, entryType, url string) {
	t.Helper()

	for _, entry := range entries {
		if entry.Name != name {
			continue
		}
		if entry.Type != entryType {
			t.Fatalf("entry %q has type %q, want %q", name, entry.Type, entryType)
		}
		if entry.URL != url {
			t.Fatalf("entry %q has URL %q, want %q", name, entry.URL, url)
		}
		return
	}

	t.Fatalf("missing entry %q", name)
}

func assertNoEntry(t *testing.T, entries []generatedEntry, name string) {
	t.Helper()

	for _, entry := range entries {
		if entry.Name == name {
			t.Fatalf("did not expect entry %q", name)
		}
	}
}

func assertFileExists(t *testing.T, filePath string) {
	t.Helper()

	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected file %q to exist: %v", filePath, err)
	}
}

func assertCatalogHasRow(t *testing.T, catalog *Catalog, query, relPath string) {
	t.Helper()

	var count int
	if err := catalog.db.QueryRow(query, relPath).Scan(&count); err != nil {
		t.Fatalf("query sqlite catalog for %q: %v", relPath, err)
	}
	if count == 0 {
		t.Fatalf("expected sqlite catalog row for %q", relPath)
	}
}
