package utilities

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Catalog struct {
	db      *sql.DB
	rootDir string
	relPath string
}

func OpenCatalog(rootDir, relPath string) (*Catalog, error) {
	if strings.TrimSpace(relPath) == "" {
		return nil, fmt.Errorf("sqlite path cannot be empty")
	}
	if filepath.IsAbs(relPath) {
		return nil, fmt.Errorf("sqlite path must be relative to the traced root")
	}

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve traced root: %w", err)
	}

	cleanRelPath := normalizeRelativePath(relPath)
	if cleanRelPath == "" {
		return nil, fmt.Errorf("sqlite path cannot resolve to the traced root")
	}

	catalogPath := filepath.Join(absRoot, filepath.FromSlash(cleanRelPath))
	resolvedRel, err := filepath.Rel(absRoot, catalogPath)
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite path: %w", err)
	}
	if strings.HasPrefix(resolvedRel, "..") {
		return nil, fmt.Errorf("sqlite path must stay within the traced root")
	}

	if err := os.MkdirAll(filepath.Dir(catalogPath), 0755); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}

	db, err := sql.Open("sqlite", catalogPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite catalog: %w", err)
	}

	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	catalog := &Catalog{
		db:      db,
		rootDir: absRoot,
		relPath: normalizeRelativePath(resolvedRel),
	}

	if err := catalog.EnsureSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return catalog, nil
}

func (c *Catalog) EnsureSchema() error {
	_, err := c.db.Exec(`
CREATE TABLE IF NOT EXISTS directories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  parent_id INTEGER REFERENCES directories(id),
  name TEXT NOT NULL,
  rel_path TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS files (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  directory_id INTEGER NOT NULL REFERENCES directories(id),
  basename TEXT NOT NULL,
  extension TEXT NOT NULL,
  rel_path TEXT NOT NULL UNIQUE,
  content_hash TEXT,
  size_bytes INTEGER,
  mtime_utc TEXT,
  is_deleted INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS file_metadata (
  file_id INTEGER PRIMARY KEY REFERENCES files(id),
  external_link TEXT,
  notes TEXT,
  title_override TEXT,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`)
	if err != nil {
		return fmt.Errorf("ensure sqlite schema: %w", err)
	}

	return nil
}

func (c *Catalog) UpsertDirectory(path string) (int64, error) {
	relPath, err := c.relativePath(path)
	if err != nil {
		return 0, err
	}

	var parentID any
	name := filepath.Base(c.rootDir)
	if relPath != "" {
		parentRel := parentRelativePath(relPath)
		resolvedParentID, err := c.UpsertDirectory(parentRel)
		if err != nil {
			return 0, err
		}
		parentID = resolvedParentID
		name = pathBase(relPath)
	}

	_, err = c.db.Exec(`
INSERT INTO directories (parent_id, name, rel_path, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(rel_path) DO UPDATE SET
  parent_id = excluded.parent_id,
  name = excluded.name,
  updated_at = CURRENT_TIMESTAMP
`, parentID, name, relPath)
	if err != nil {
		return 0, fmt.Errorf("upsert sqlite directory %q: %w", relPath, err)
	}

	var directoryID int64
	err = c.db.QueryRow(`SELECT id FROM directories WHERE rel_path = ?`, relPath).Scan(&directoryID)
	if err != nil {
		return 0, fmt.Errorf("read sqlite directory %q: %w", relPath, err)
	}

	return directoryID, nil
}

func (c *Catalog) UpsertFiles(dirPath string, files []File) error {
	if len(files) == 0 {
		return nil
	}

	directoryRelPath, err := c.relativePath(dirPath)
	if err != nil {
		return err
	}

	directoryID, err := c.UpsertDirectory(directoryRelPath)
	if err != nil {
		return err
	}

	absoluteDirectory := filepath.Join(c.rootDir, filepath.FromSlash(directoryRelPath))
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("start sqlite transaction: %w", err)
	}
	defer tx.Rollback()

	for _, file := range files {
		relPath := joinRelativePath(directoryRelPath, file.Name)
		if c.IsCatalogFile(relPath) {
			continue
		}

		filePath := filepath.Join(absoluteDirectory, file.Name)
		info, err := os.Stat(filePath)
		if err != nil {
			return fmt.Errorf("stat %q for sqlite sync: %w", relPath, err)
		}

		_, err = tx.Exec(`
INSERT INTO files (directory_id, basename, extension, rel_path, size_bytes, mtime_utc, is_deleted, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 0, CURRENT_TIMESTAMP)
ON CONFLICT(rel_path) DO UPDATE SET
  directory_id = excluded.directory_id,
  basename = excluded.basename,
  extension = excluded.extension,
  size_bytes = excluded.size_bytes,
  mtime_utc = excluded.mtime_utc,
  is_deleted = 0,
  updated_at = CURRENT_TIMESTAMP
`, directoryID, file.Name, filepath.Ext(file.Name), relPath, info.Size(), info.ModTime().UTC().Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("upsert sqlite file %q: %w", relPath, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite transaction: %w", err)
	}

	return nil
}

func (c *Catalog) GetExternalLinks(dirPath string) (map[string]string, error) {
	directoryRelPath, err := c.relativePath(dirPath)
	if err != nil {
		return nil, err
	}

	rows, err := c.db.Query(`
SELECT f.basename, m.external_link
FROM files AS f
JOIN directories AS d ON d.id = f.directory_id
JOIN file_metadata AS m ON m.file_id = f.id
WHERE d.rel_path = ?
  AND f.is_deleted = 0
  AND COALESCE(m.external_link, '') <> ''
`, directoryRelPath)
	if err != nil {
		return nil, fmt.Errorf("read sqlite links for %q: %w", directoryRelPath, err)
	}
	defer rows.Close()

	links := make(map[string]string)
	for rows.Next() {
		var basename, externalLink string
		if err := rows.Scan(&basename, &externalLink); err != nil {
			return nil, fmt.Errorf("scan sqlite link for %q: %w", directoryRelPath, err)
		}
		if externalLink == "" {
			continue
		}

		id := strings.TrimSuffix(basename, filepath.Ext(basename))
		links[id] = externalLink
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite links for %q: %w", directoryRelPath, err)
	}

	return links, nil
}

func (c *Catalog) GetSearchEntries() ([]SearchEntry, error) {
	if c == nil {
		return nil, fmt.Errorf("sqlite catalog is not initialized")
	}

	var entries []SearchEntry

	directoryRows, err := c.db.Query(`
SELECT rel_path
FROM directories
WHERE rel_path <> ''
ORDER BY rel_path COLLATE NOCASE
`)
	if err != nil {
		return nil, fmt.Errorf("read sqlite directories for search: %w", err)
	}
	defer directoryRows.Close()

	for directoryRows.Next() {
		var relPath string
		if err := directoryRows.Scan(&relPath); err != nil {
			return nil, fmt.Errorf("scan sqlite search directory: %w", err)
		}
		entries = append(entries, SearchEntry{
			Name: pathBase(relPath),
			Type: "d",
			Path: parentRelativePath(relPath),
		})
	}
	if err := directoryRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite directories for search: %w", err)
	}

	fileRows, err := c.db.Query(`
SELECT basename, rel_path
FROM files
WHERE is_deleted = 0
ORDER BY rel_path COLLATE NOCASE
`)
	if err != nil {
		return nil, fmt.Errorf("read sqlite files for search: %w", err)
	}
	defer fileRows.Close()

	for fileRows.Next() {
		var basename, relPath string
		if err := fileRows.Scan(&basename, &relPath); err != nil {
			return nil, fmt.Errorf("scan sqlite search file: %w", err)
		}
		if c.IsCatalogFile(relPath) {
			continue
		}
		entries = append(entries, SearchEntry{
			Name: basename,
			Type: "f",
			Path: parentRelativePath(relPath),
		})
	}
	if err := fileRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite files for search: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path != entries[j].Path {
			return entries[i].Path < entries[j].Path
		}
		if entries[i].Type != entries[j].Type {
			return entries[i].Type < entries[j].Type
		}
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

func (c *Catalog) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

func (c *Catalog) IsCatalogFile(relPath string) bool {
	if c == nil {
		return false
	}
	return normalizeRelativePath(relPath) == c.relPath
}

func (c *Catalog) relativePath(path string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("sqlite catalog is not initialized")
	}

	if strings.TrimSpace(path) == "" || path == "." {
		return "", nil
	}

	absolutePath := path
	if !filepath.IsAbs(absolutePath) {
		absolutePath = filepath.Join(c.rootDir, path)
	}

	absolutePath, err := filepath.Abs(absolutePath)
	if err != nil {
		return "", fmt.Errorf("resolve relative sqlite path %q: %w", path, err)
	}

	relPath, err := filepath.Rel(c.rootDir, absolutePath)
	if err != nil {
		return "", fmt.Errorf("calculate sqlite relative path %q: %w", path, err)
	}
	if relPath == "." {
		return "", nil
	}
	if strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("path %q is outside the traced root", path)
	}

	return normalizeRelativePath(relPath), nil
}

func normalizeRelativePath(path string) string {
	cleaned := filepath.Clean(path)
	if cleaned == "." {
		return ""
	}
	return filepath.ToSlash(cleaned)
}

func joinRelativePath(directory, name string) string {
	if directory == "" {
		return normalizeRelativePath(name)
	}
	return normalizeRelativePath(filepath.Join(directory, name))
}

func parentRelativePath(path string) string {
	if path == "" {
		return ""
	}

	separator := strings.LastIndex(path, "/")
	if separator == -1 {
		return ""
	}
	return path[:separator]
}

func pathBase(path string) string {
	separator := strings.LastIndex(path, "/")
	if separator == -1 {
		return path
	}
	return path[separator+1:]
}
