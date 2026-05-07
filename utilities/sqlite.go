package utilities

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type Catalog struct {
	db      *sql.DB
	rootDir string
	relPath string
	tx      *sql.Tx
	dirIDs  map[string]int64
}

const catalogSchemaVersion = 3

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
		dirIDs:  make(map[string]int64),
	}

	if err := catalog.EnsureSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return catalog, nil
}

func (c *Catalog) EnsureSchema() error {
	if err := c.migrateSchema(); err != nil {
		return fmt.Errorf("ensure sqlite schema: %w", err)
	}

	return nil
}

func (c *Catalog) migrateSchema() error {
	hasDirectories, err := c.tableExists("directories")
	if err != nil {
		return err
	}
	hasFiles, err := c.tableExists("files")
	if err != nil {
		return err
	}
	hasMetadata, err := c.tableExists("file_metadata")
	if err != nil {
		return err
	}

	if !hasDirectories && !hasFiles && !hasMetadata {
		return c.createSchema()
	}

	legacyDirectories, err := c.hasLegacyRelPathUniqueIndex("directories")
	if err != nil {
		return err
	}
	legacyFiles, err := c.hasLegacyRelPathUniqueIndex("files")
	if err != nil {
		return err
	}

	if legacyDirectories || legacyFiles {
		return c.rebuildLegacySchema()
	}

	hasFileMtime, err := c.hasColumn("files", "mtime_utc")
	if err != nil {
		return err
	}
	if hasFileMtime {
		return c.rebuildFilesWithoutMtime()
	}

	_, err = c.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, catalogSchemaVersion))
	if err != nil {
		return fmt.Errorf("set sqlite schema version: %w", err)
	}

	return nil
}

func (c *Catalog) createSchema() error {
	_, err := c.db.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS directories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  parent_id INTEGER REFERENCES directories(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  rel_path TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(rel_path, name)
);

CREATE TABLE IF NOT EXISTS files (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  directory_id INTEGER NOT NULL REFERENCES directories(id) ON DELETE CASCADE,
  basename TEXT NOT NULL,
  extension TEXT NOT NULL,
  rel_path TEXT NOT NULL,
  content_hash TEXT,
  size_bytes INTEGER,
  is_deleted INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(rel_path, basename)
);

CREATE TABLE IF NOT EXISTS file_metadata (
  file_id INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  external_link TEXT,
  notes TEXT,
  title_override TEXT,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_directories_parent_name ON directories(parent_id, name);
CREATE INDEX IF NOT EXISTS idx_files_directory_basename ON files(directory_id, basename);
CREATE INDEX IF NOT EXISTS idx_files_rel_path ON files(rel_path);
PRAGMA user_version = %d;
`, catalogSchemaVersion))
	if err != nil {
		return fmt.Errorf("create sqlite schema: %w", err)
	}

	return nil
}

func (c *Catalog) rebuildLegacySchema() error {
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("start sqlite schema migration: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(fmt.Sprintf(`
ALTER TABLE file_metadata RENAME TO file_metadata_legacy;
ALTER TABLE files RENAME TO files_legacy;
ALTER TABLE directories RENAME TO directories_legacy;

CREATE TABLE directories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  parent_id INTEGER REFERENCES directories(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  rel_path TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(rel_path, name)
);

CREATE TABLE files (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  directory_id INTEGER NOT NULL REFERENCES directories(id) ON DELETE CASCADE,
  basename TEXT NOT NULL,
  extension TEXT NOT NULL,
  rel_path TEXT NOT NULL,
  content_hash TEXT,
  size_bytes INTEGER,
  is_deleted INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(rel_path, basename)
);

CREATE TABLE file_metadata (
  file_id INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  external_link TEXT,
  notes TEXT,
  title_override TEXT,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_directories_parent_name ON directories(parent_id, name);
CREATE INDEX idx_files_directory_basename ON files(directory_id, basename);
CREATE INDEX idx_files_rel_path ON files(rel_path);

INSERT INTO directories (id, parent_id, name, rel_path, created_at, updated_at)
SELECT
  id,
  parent_id,
  name,
  CASE
    WHEN rel_path = '' THEN ''
    WHEN INSTR(rel_path, '/') = 0 THEN ''
    ELSE SUBSTR(rel_path, 1, LENGTH(rel_path) - LENGTH(name) - 1) || '/'
  END,
  created_at,
  updated_at
FROM directories_legacy;

INSERT INTO files (id, directory_id, basename, extension, rel_path, content_hash, size_bytes, is_deleted, created_at, updated_at)
SELECT
  id,
  directory_id,
  basename,
  extension,
  CASE
    WHEN rel_path = '' THEN ''
    WHEN INSTR(rel_path, '/') = 0 THEN ''
    ELSE SUBSTR(rel_path, 1, LENGTH(rel_path) - LENGTH(basename))
  END,
  content_hash,
  size_bytes,
  is_deleted,
  created_at,
  updated_at
FROM files_legacy;

INSERT INTO file_metadata (file_id, external_link, notes, title_override, updated_at)
SELECT file_id, external_link, notes, title_override, updated_at
FROM file_metadata_legacy;

DROP TABLE file_metadata_legacy;
DROP TABLE files_legacy;
DROP TABLE directories_legacy;

PRAGMA user_version = %d;
`, catalogSchemaVersion))
	if err != nil {
		return fmt.Errorf("migrate sqlite schema: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite schema migration: %w", err)
	}

	return nil
}

func (c *Catalog) rebuildFilesWithoutMtime() error {
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("start sqlite files migration: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(fmt.Sprintf(`
ALTER TABLE files RENAME TO files_legacy;
DROP INDEX IF EXISTS idx_files_directory_basename;
DROP INDEX IF EXISTS idx_files_rel_path;

CREATE TABLE files (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	directory_id INTEGER NOT NULL REFERENCES directories(id) ON DELETE CASCADE,
	basename TEXT NOT NULL,
	extension TEXT NOT NULL,
	rel_path TEXT NOT NULL,
	content_hash TEXT,
	size_bytes INTEGER,
	is_deleted INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(rel_path, basename)
);

CREATE INDEX idx_files_directory_basename ON files(directory_id, basename);
CREATE INDEX idx_files_rel_path ON files(rel_path);

INSERT INTO files (id, directory_id, basename, extension, rel_path, content_hash, size_bytes, is_deleted, created_at, updated_at)
SELECT
	id,
	directory_id,
	basename,
	extension,
	rel_path,
	content_hash,
	size_bytes,
	is_deleted,
	created_at,
	updated_at
FROM files_legacy;

DROP TABLE files_legacy;

PRAGMA user_version = %d;
`, catalogSchemaVersion))
	if err != nil {
		return fmt.Errorf("migrate sqlite files schema: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite files migration: %w", err)
	}

	return nil
}

func (c *Catalog) tableExists(name string) (bool, error) {
	var count int
	err := c.db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check sqlite table %q: %w", name, err)
	}

	return count > 0, nil
}

func (c *Catalog) hasColumn(tableName, columnName string) (bool, error) {
	rows, err := c.db.Query(fmt.Sprintf(`PRAGMA table_info(%q)`, tableName))
	if err != nil {
		return false, fmt.Errorf("inspect sqlite columns for %q: %w", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("scan sqlite column for %q: %w", tableName, err)
		}
		if name == columnName {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate sqlite columns for %q: %w", tableName, err)
	}

	return false, nil
}

func (c *Catalog) hasLegacyRelPathUniqueIndex(tableName string) (bool, error) {
	rows, err := c.db.Query(fmt.Sprintf(`PRAGMA index_list(%q)`, tableName))
	if err != nil {
		return false, fmt.Errorf("list sqlite indexes for %q: %w", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var sequence int
		var name string
		var unique int
		var origin, partial string
		if err := rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			return false, fmt.Errorf("scan sqlite index for %q: %w", tableName, err)
		}
		if unique == 0 {
			continue
		}

		legacy, err := c.indexMatchesSingleColumn(name, "rel_path")
		if err != nil {
			return false, err
		}
		if legacy {
			return true, nil
		}
	}

	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate sqlite indexes for %q: %w", tableName, err)
	}

	return false, nil
}

func (c *Catalog) indexMatchesSingleColumn(indexName, column string) (bool, error) {
	rows, err := c.db.Query(fmt.Sprintf(`PRAGMA index_info(%q)`, indexName))
	if err != nil {
		return false, fmt.Errorf("inspect sqlite index %q: %w", indexName, err)
	}
	defer rows.Close()

	columns := make([]string, 0, 2)
	for rows.Next() {
		var sequenceNumber, columnPosition int
		var columnName string
		if err := rows.Scan(&sequenceNumber, &columnPosition, &columnName); err != nil {
			return false, fmt.Errorf("scan sqlite index column for %q: %w", indexName, err)
		}
		columns = append(columns, columnName)
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate sqlite index columns for %q: %w", indexName, err)
	}

	return len(columns) == 1 && columns[0] == column, nil
}

func (c *Catalog) WithSyncTransaction(fn func() error) error {
	if c == nil {
		return fmt.Errorf("sqlite catalog is not initialized")
	}
	if c.tx != nil {
		return fn()
	}

	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("start sqlite sync transaction: %w", err)
	}

	previousTx := c.tx
	previousDirIDs := c.dirIDs
	c.tx = tx
	c.dirIDs = make(map[string]int64, len(previousDirIDs))
	for key, value := range previousDirIDs {
		c.dirIDs[key] = value
	}

	defer func() {
		c.tx = previousTx
		c.dirIDs = previousDirIDs
	}()

	if err := fn(); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("rollback sqlite sync transaction after %v: %w", err, rollbackErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite sync transaction: %w", err)
	}

	for key, value := range c.dirIDs {
		previousDirIDs[key] = value
	}

	return nil
}

func (c *Catalog) UpsertDirectory(path string) (int64, error) {
	relPath, err := c.relativePath(path)
	if err != nil {
		return 0, err
	}
	if directoryID, ok := c.dirIDs[relPath]; ok {
		return directoryID, nil
	}

	var parentID any
	name, storedRelPath := c.directoryStorage(relPath)
	if relPath != "" {
		parentRel := parentRelativePath(relPath)
		resolvedParentID, err := c.UpsertDirectory(parentRel)
		if err != nil {
			return 0, err
		}
		parentID = resolvedParentID
	}

	execTarget := c.execTarget()
	_, err = execTarget.Exec(`
INSERT INTO directories (parent_id, name, rel_path, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(rel_path, name) DO UPDATE SET
  parent_id = excluded.parent_id,
  name = excluded.name,
  updated_at = CURRENT_TIMESTAMP
`, parentID, name, storedRelPath)
	if err != nil {
		return 0, fmt.Errorf("upsert sqlite directory %q: %w", relPath, err)
	}

	var directoryID int64
	err = c.queryRow(`SELECT id FROM directories WHERE rel_path = ? AND name = ?`, storedRelPath, name).Scan(&directoryID)
	if err != nil {
		return 0, fmt.Errorf("read sqlite directory %q: %w", relPath, err)
	}
	c.dirIDs[relPath] = directoryID

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
	execTarget := c.execTarget()
	var tx *sql.Tx
	if c.tx == nil {
		tx, err = c.db.Begin()
		if err != nil {
			return fmt.Errorf("start sqlite transaction: %w", err)
		}
		execTarget = tx
		defer tx.Rollback()
	}

	for _, file := range files {
		actualRelPath := joinRelativePath(directoryRelPath, file.Name)
		if c.IsCatalogFile(actualRelPath) {
			continue
		}

		filePath := filepath.Join(absoluteDirectory, file.Name)
		info, err := os.Stat(filePath)
		if err != nil {
			return fmt.Errorf("stat %q for sqlite sync: %w", actualRelPath, err)
		}

		_, err = execTarget.Exec(`
INSERT INTO files (directory_id, basename, extension, rel_path, size_bytes, is_deleted, updated_at)
VALUES (?, ?, ?, ?, ?, 0, CURRENT_TIMESTAMP)
ON CONFLICT(rel_path, basename) DO UPDATE SET
  directory_id = excluded.directory_id,
  basename = excluded.basename,
  extension = excluded.extension,
  size_bytes = excluded.size_bytes,
  is_deleted = 0,
  updated_at = CURRENT_TIMESTAMP
		`, directoryID, file.Name, filepath.Ext(file.Name), storedFileRelPath(directoryRelPath), info.Size())
		if err != nil {
			return fmt.Errorf("upsert sqlite file %q: %w", actualRelPath, err)
		}
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit sqlite transaction: %w", err)
		}
	}

	return nil
}

func (c *Catalog) GetExternalLinks(dirPath string) (map[string]string, error) {
	directoryRelPath, err := c.relativePath(dirPath)
	if err != nil {
		return nil, err
	}
	directoryID, err := c.directoryIDForPath(directoryRelPath)
	if err != nil {
		return nil, err
	}

	rows, err := c.db.Query(`
SELECT f.basename, m.external_link
FROM files AS f
JOIN file_metadata AS m ON m.file_id = f.id
WHERE f.directory_id = ?
  AND f.is_deleted = 0
  AND COALESCE(m.external_link, '') <> ''
`, directoryID)
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
SELECT name, rel_path
FROM directories
WHERE parent_id IS NOT NULL
ORDER BY rel_path COLLATE NOCASE, name COLLATE NOCASE
`)
	if err != nil {
		return nil, fmt.Errorf("read sqlite directories for search: %w", err)
	}
	defer directoryRows.Close()

	for directoryRows.Next() {
		var name, relPath string
		if err := directoryRows.Scan(&name, &relPath); err != nil {
			return nil, fmt.Errorf("scan sqlite search directory: %w", err)
		}
		entries = append(entries, SearchEntry{
			Name: name,
			Type: "d",
			Path: displayPath(relPath),
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
		if c.IsCatalogFile(joinStoredRelativePath(relPath, basename)) {
			continue
		}
		entries = append(entries, SearchEntry{
			Name: basename,
			Type: "f",
			Path: displayPath(relPath),
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

func (c *Catalog) Vacuum() error {
	if c == nil {
		return fmt.Errorf("sqlite catalog is not initialized")
	}
	if c.tx != nil {
		return fmt.Errorf("sqlite vacuum must run outside a transaction")
	}
	if _, err := c.db.Exec(`VACUUM`); err != nil {
		return fmt.Errorf("vacuum sqlite catalog: %w", err)
	}

	return nil
}

type sqlExecTarget interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (c *Catalog) execTarget() sqlExecTarget {
	if c != nil && c.tx != nil {
		return c.tx
	}
	return c.db
}

func (c *Catalog) queryRow(query string, args ...any) *sql.Row {
	if c != nil && c.tx != nil {
		return c.tx.QueryRow(query, args...)
	}
	return c.db.QueryRow(query, args...)
}

func (c *Catalog) query(query string, args ...any) (*sql.Rows, error) {
	if c != nil && c.tx != nil {
		return c.tx.Query(query, args...)
	}
	return c.db.Query(query, args...)
}

func (c *Catalog) IsCatalogFile(relPath string) bool {
	if c == nil {
		return false
	}
	normalized := normalizeRelativePath(relPath)
	return normalized == c.relPath || strings.HasPrefix(normalized, c.relPath+"-")
}

func (c *Catalog) PruneMissingEntries() error {
	if c == nil {
		return fmt.Errorf("sqlite catalog is not initialized")
	}

	execTarget := c.execTarget()
	var tx *sql.Tx
	var err error
	if c.tx == nil {
		tx, err = c.db.Begin()
		if err != nil {
			return fmt.Errorf("start sqlite prune transaction: %w", err)
		}
		execTarget = tx
		defer tx.Rollback()
	}

	staleFileIDs, err := c.findMissingFileIDs()
	if err != nil {
		return err
	}
	for _, id := range staleFileIDs {
		if _, err := execTarget.Exec(`DELETE FROM files WHERE id = ?`, id); err != nil {
			return fmt.Errorf("delete sqlite file %d: %w", id, err)
		}
	}

	staleDirectoryIDs, err := c.findMissingDirectoryIDs()
	if err != nil {
		return err
	}
	for _, id := range staleDirectoryIDs {
		if _, err := execTarget.Exec(`DELETE FROM directories WHERE id = ?`, id); err != nil {
			return fmt.Errorf("delete sqlite directory %d: %w", id, err)
		}
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit sqlite prune transaction: %w", err)
		}
	}

	return nil
}

func (c *Catalog) findMissingFileIDs() ([]int64, error) {
	rows, err := c.query(`SELECT id, rel_path, basename FROM files`)
	if err != nil {
		return nil, fmt.Errorf("read sqlite files for pruning: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		var relPath, basename string
		if err := rows.Scan(&id, &relPath, &basename); err != nil {
			return nil, fmt.Errorf("scan sqlite file for pruning: %w", err)
		}

		actualRelPath := joinStoredRelativePath(relPath, basename)
		if c.IsCatalogFile(actualRelPath) {
			continue
		}
		if _, err := os.Stat(filepath.Join(c.rootDir, filepath.FromSlash(actualRelPath))); err != nil {
			if os.IsNotExist(err) {
				ids = append(ids, id)
				continue
			}
			return nil, fmt.Errorf("stat %q for sqlite prune: %w", actualRelPath, err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite files for pruning: %w", err)
	}

	return ids, nil
}

func (c *Catalog) findMissingDirectoryIDs() ([]int64, error) {
	rows, err := c.query(`
SELECT id, rel_path, name
FROM directories
WHERE parent_id IS NOT NULL
ORDER BY LENGTH(rel_path) + LENGTH(name) DESC, name DESC
`)
	if err != nil {
		return nil, fmt.Errorf("read sqlite directories for pruning: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		var relPath, name string
		if err := rows.Scan(&id, &relPath, &name); err != nil {
			return nil, fmt.Errorf("scan sqlite directory for pruning: %w", err)
		}

		actualRelPath := joinStoredRelativePath(relPath, name)
		if _, err := os.Stat(filepath.Join(c.rootDir, filepath.FromSlash(actualRelPath))); err != nil {
			if os.IsNotExist(err) {
				ids = append(ids, id)
				continue
			}
			return nil, fmt.Errorf("stat %q for sqlite prune: %w", actualRelPath, err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite directories for pruning: %w", err)
	}

	return ids, nil
}

func (c *Catalog) directoryIDForPath(path string) (int64, error) {
	if directoryID, ok := c.dirIDs[path]; ok {
		return directoryID, nil
	}

	name, relPath := c.directoryStorage(path)
	var directoryID int64
	err := c.queryRow(`SELECT id FROM directories WHERE rel_path = ? AND name = ?`, relPath, name).Scan(&directoryID)
	if err != nil {
		return 0, fmt.Errorf("read sqlite directory %q: %w", path, err)
	}
	c.dirIDs[path] = directoryID

	return directoryID, nil
}

func (c *Catalog) directoryStorage(path string) (string, string) {
	if path == "" {
		return filepath.Base(c.rootDir), ""
	}
	return pathBase(path), parentStoredRelPath(path)
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

func joinStoredRelativePath(relPath, name string) string {
	return joinRelativePath(strings.TrimSuffix(relPath, "/"), name)
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

func parentStoredRelPath(path string) string {
	parent := parentRelativePath(path)
	if parent == "" {
		return ""
	}
	return parent + "/"
}

func storedFileRelPath(directory string) string {
	if directory == "" {
		return ""
	}
	return normalizeRelativePath(directory) + "/"
}

func displayPath(path string) string {
	return strings.TrimSuffix(path, "/")
}

func pathBase(path string) string {
	separator := strings.LastIndex(path, "/")
	if separator == -1 {
		return path
	}
	return path[separator+1:]
}
