package utilities

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
		IndexFolder(".", GenerateBoilerplateHTML("Fixture", "", ""), 0, nil, true, false, true, false, nil, true)
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
		if err := catalog.WithSyncTransaction(func() error {
			IndexFolder(".", GenerateBoilerplateHTML("Fixture", "", ""), 0, nil, true, false, true, false, catalog, true)
			return nil
		}); err != nil {
			t.Fatalf("sync SQLite catalog in transaction: %v", err)
		}
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

func TestWriteSearchPageUsesSQLiteCatalog(t *testing.T) {
	root := copyFixtureToTempDir(t)
	catalog, err := OpenCatalog(root, "catalog.db")
	if err != nil {
		t.Fatalf("open sqlite catalog: %v", err)
	}
	defer catalog.Close()

	runFromDir(t, root, func() {
		if err := catalog.WithSyncTransaction(func() error {
			IndexFolder(".", GenerateBoilerplateHTML("Fixture", "", ""), 0, nil, true, false, true, true, catalog, true)
			return nil
		}); err != nil {
			t.Fatalf("sync SQLite catalog in transaction: %v", err)
		}
		if err := WriteSearchPage(GenerateSearchHTML("Fixture", "", ""), catalog); err != nil {
			t.Fatalf("write SQLite-backed search page: %v", err)
		}
	})

	searchHTML, err := os.ReadFile(filepath.Join(root, "search.html"))
	if err != nil {
		t.Fatalf("read generated search page: %v", err)
	}

	content := string(searchHTML)
	assertContainsText(t, content, `const sd = [`)
	assertContainsText(t, content, `{"n":"albums","t":"d","p":""}`)
	assertContainsText(t, content, `{"n":"guide.txt","t":"f","p":"docs"}`)
	assertContainsText(t, content, `{"n":"encore.txt","t":"f","p":"albums/live"}`)
	assertNotContainsText(t, content, `catalog.db`)
}

func TestWriteSearchPageRequiresSQLiteCatalog(t *testing.T) {
	err := WriteSearchPage(GenerateSearchHTML("Fixture", "", ""), nil)
	if err == nil {
		t.Fatal("expected SQLite-backed search page generation to require a catalog")
	}
	if !strings.Contains(err.Error(), "sqlite catalog is required") {
		t.Fatalf("unexpected search page error: %v", err)
	}
}

func TestIndexFolderSQLiteOnlySkipsGeneratedFiles(t *testing.T) {
	root := copyFixtureToTempDir(t)
	catalog, err := OpenCatalog(root, "catalog.db")
	if err != nil {
		t.Fatalf("open sqlite catalog: %v", err)
	}
	defer catalog.Close()

	runFromDir(t, root, func() {
		if err := catalog.WithSyncTransaction(func() error {
			IndexFolder(".", GenerateBoilerplateHTML("Fixture", "", ""), 0, nil, true, false, true, true, catalog, false)
			return nil
		}); err != nil {
			t.Fatalf("sync SQLite catalog in transaction: %v", err)
		}
	})

	assertFileDoesNotExist(t, filepath.Join(root, "index.html"))
	assertFileDoesNotExist(t, filepath.Join(root, "data.json"))
	assertFileDoesNotExist(t, filepath.Join(root, "search.html"))
	assertFileDoesNotExist(t, filepath.Join(root, "albums", "index.html"))
	assertFileDoesNotExist(t, filepath.Join(root, "albums", "data.json"))

	assertCatalogHasRow(t, catalog, `SELECT COUNT(1) FROM directories WHERE rel_path = ?`, "albums")
	assertCatalogHasRow(t, catalog, `SELECT COUNT(1) FROM files WHERE rel_path = ?`, "song.mp3")
	assertCatalogHasRow(t, catalog, `SELECT COUNT(1) FROM files WHERE rel_path = ?`, "albums/live/encore.txt")
	assertCatalogHasRow(t, catalog, `SELECT COUNT(1) FROM files WHERE rel_path = ?`, "docs/guide.txt")
	assertCatalogDoesNotHaveRow(t, catalog, `SELECT COUNT(1) FROM files WHERE rel_path = ?`, "index.html")
}

func copyFixtureToTempDir(t *testing.T) string {
	t.Helper()

	root := t.TempDir()

	fixtureFiles := map[string]string{
		"welcome.txt":            "Welcome to the fixture root.\n",
		"song.mp3":               "Placeholder audio content.\n",
		"metadata.json":          "[\n  {\n    \"n\": \"song\",\n    \"u\": \"https://fallback.example/root-song\"\n  }\n]\n",
		"docs/guide.txt":         "Fixture documentation.\n",
		"albums/track.mp3":       "Placeholder album audio.\n",
		"albums/metadata.json":   "[\n  {\n    \"n\": \"track\",\n    \"u\": \"https://fallback.example/album-track\"\n  }\n]\n",
		"albums/live/encore.txt": "Encore notes.\n",
	}

	for relPath, content := range fixtureFiles {
		targetPath := filepath.Join(root, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			t.Fatalf("create fixture directory for %q: %v", relPath, err)
		}
		if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
			t.Fatalf("write fixture file %q: %v", relPath, err)
		}
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

func assertFileDoesNotExist(t *testing.T, filePath string) {
	t.Helper()

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected file %q to be absent, got err=%v", filePath, err)
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

func assertCatalogDoesNotHaveRow(t *testing.T, catalog *Catalog, query, relPath string) {
	t.Helper()

	var count int
	if err := catalog.db.QueryRow(query, relPath).Scan(&count); err != nil {
		t.Fatalf("query sqlite catalog for %q: %v", relPath, err)
	}
	if count != 0 {
		t.Fatalf("did not expect sqlite catalog row for %q", relPath)
	}
}

func assertContainsText(t *testing.T, text, want string) {
	t.Helper()

	if !strings.Contains(text, want) {
		t.Fatalf("expected generated content to contain %q", want)
	}
}

func assertNotContainsText(t *testing.T, text, unwanted string) {
	t.Helper()

	if strings.Contains(text, unwanted) {
		t.Fatalf("did not expect generated content to contain %q", unwanted)
	}
}
