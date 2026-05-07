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

type CatalogOptions struct {
	IncludeTimestamps bool
}

type Catalog struct {
	db      *sql.DB
	rootDir string
	relPath string
	tx      *sql.Tx
	dirIDs  map[string]int64
	options CatalogOptions
}

type columnInfo struct {
	name        string
	dataType    string
	notNull     int
	primaryKey  int
	defaultExpr sql.NullString
}

type schemaState struct {
	hasDirectories      bool
	hasFiles            bool
	hasMetadata         bool
	legacyDirectoryPath bool
	directoryColumns    map[string]columnInfo
	fileColumns         map[string]columnInfo
	metadataColumns     map[string]columnInfo
}

const (
	catalogSchemaVersion               = 4
	catalogSchemaVersionWithTimestamps = 5
)

func OpenCatalog(rootDir, relPath string, options CatalogOptions) (*Catalog, error) {
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
		options: options,
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
	state, err := c.inspectSchemaState()
	if err != nil {
		return err
	}

	if !state.hasDirectories && !state.hasFiles && !state.hasMetadata {
		return c.createSchema()
	}

	needsRebuild, err := c.schemaNeedsRebuild(state)
	if err != nil {
		return err
	}
	if needsRebuild {
		return c.rebuildSchema(state)
	}

	if err := c.dropRedundantIndexes(); err != nil {
		return err
	}

	if _, err := c.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, c.targetSchemaVersion())); err != nil {
		return fmt.Errorf("set sqlite schema version: %w", err)
	}

	return nil
}

func (c *Catalog) createSchema() error {
	if _, err := c.db.Exec(c.schemaSQL()); err != nil {
		return fmt.Errorf("create sqlite schema: %w", err)
	}

	return nil
}

func (c *Catalog) rebuildSchema(state schemaState) error {
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("start sqlite schema migration: %w", err)
	}
	defer tx.Rollback()

	if state.hasMetadata {
		if _, err := tx.Exec(`ALTER TABLE file_metadata RENAME TO file_metadata_legacy`); err != nil {
			return fmt.Errorf("rename sqlite metadata table: %w", err)
		}
	}
	if state.hasFiles {
		if _, err := tx.Exec(`ALTER TABLE files RENAME TO files_legacy`); err != nil {
			return fmt.Errorf("rename sqlite files table: %w", err)
		}
	}
	if state.hasDirectories {
		if _, err := tx.Exec(`ALTER TABLE directories RENAME TO directories_legacy`); err != nil {
			return fmt.Errorf("rename sqlite directories table: %w", err)
		}
	}

	if err := c.dropRedundantIndexesWithTarget(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(c.schemaSQL()); err != nil {
		return fmt.Errorf("recreate sqlite schema: %w", err)
	}

	if state.hasDirectories {
		if _, err := tx.Exec(c.copyDirectoriesSQL(state)); err != nil {
			return fmt.Errorf("copy sqlite directories: %w", err)
		}
	}
	if state.hasFiles {
		if _, err := tx.Exec(c.copyFilesSQL(state)); err != nil {
			return fmt.Errorf("copy sqlite files: %w", err)
		}
	}
	if state.hasMetadata {
		if _, err := tx.Exec(c.copyMetadataSQL(state)); err != nil {
			return fmt.Errorf("copy sqlite metadata: %w", err)
		}
	}

	if state.hasMetadata {
		if _, err := tx.Exec(`DROP TABLE file_metadata_legacy`); err != nil {
			return fmt.Errorf("drop legacy sqlite metadata table: %w", err)
		}
	}
	if state.hasFiles {
		if _, err := tx.Exec(`DROP TABLE files_legacy`); err != nil {
			return fmt.Errorf("drop legacy sqlite files table: %w", err)
		}
	}
	if state.hasDirectories {
		if _, err := tx.Exec(`DROP TABLE directories_legacy`); err != nil {
			return fmt.Errorf("drop legacy sqlite directories table: %w", err)
		}
	}

	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, c.targetSchemaVersion())); err != nil {
		return fmt.Errorf("set migrated sqlite schema version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite schema migration: %w", err)
	}

	return nil
}

func (c *Catalog) schemaNeedsRebuild(state schemaState) (bool, error) {
	currentVersion, err := c.userVersion()
	if err != nil {
		return false, err
	}
	if currentVersion != c.targetSchemaVersion() {
		return true, nil
	}
	if state.legacyDirectoryPath {
		return true, nil
	}
	if state.hasFiles {
		for _, columnName := range []string{"rel_path", "content_hash", "mtime_utc"} {
			if _, ok := state.fileColumns[columnName]; ok {
				return true, nil
			}
		}
	}

	timestampTables := []map[string]columnInfo{
		state.directoryColumns,
		state.fileColumns,
		state.metadataColumns,
	}
	for _, columns := range timestampTables {
		if c.options.IncludeTimestamps {
			if !columnsHaveIntegerTimestamps(columns) {
				return true, nil
			}
			continue
		}
		if columnsHaveAnyTimestamps(columns) {
			return true, nil
		}
	}

	for _, tableName := range []string{"directories", "files"} {
		hasAutoIncrement, err := c.tableSQLContains(tableName, "AUTOINCREMENT")
		if err != nil {
			return false, err
		}
		if hasAutoIncrement {
			return true, nil
		}
	}

	hasWithoutRowID, err := c.tableSQLContains("file_metadata", "WITHOUT ROWID")
	if err != nil {
		return false, err
	}
	if !hasWithoutRowID {
		return true, nil
	}

	return false, nil
}

func (c *Catalog) inspectSchemaState() (schemaState, error) {
	var state schemaState
	var err error

	state.hasDirectories, err = c.tableExists("directories")
	if err != nil {
		return state, err
	}
	state.hasFiles, err = c.tableExists("files")
	if err != nil {
		return state, err
	}
	state.hasMetadata, err = c.tableExists("file_metadata")
	if err != nil {
		return state, err
	}

	if state.hasDirectories {
		state.directoryColumns, err = c.tableColumns("directories")
		if err != nil {
			return state, err
		}
		state.legacyDirectoryPath, err = c.hasLegacyRelPathUniqueIndex("directories")
		if err != nil {
			return state, err
		}
	}
	if state.hasFiles {
		state.fileColumns, err = c.tableColumns("files")
		if err != nil {
			return state, err
		}
	}
	if state.hasMetadata {
		state.metadataColumns, err = c.tableColumns("file_metadata")
		if err != nil {
			return state, err
		}
	}

	return state, nil
}

func (c *Catalog) copyDirectoriesSQL(state schemaState) string {
	relPathExpr := `rel_path`
	if state.legacyDirectoryPath {
		relPathExpr = `CASE
  WHEN rel_path = '' THEN ''
  WHEN INSTR(rel_path, '/') = 0 THEN ''
  ELSE SUBSTR(rel_path, 1, LENGTH(rel_path) - LENGTH(name) - 1) || '/'
END`
	}

	columns := []string{"id", "parent_id", "name", "rel_path"}
	values := []string{"id", "parent_id", "name", relPathExpr}
	if c.options.IncludeTimestamps {
		createdAt := c.timestampValueExpression("created_at", state.directoryColumns, `unixepoch()`)
		columns = append(columns, "created_at", "updated_at")
		values = append(values,
			createdAt,
			c.timestampValueExpression("updated_at", state.directoryColumns, createdAt),
		)
	}

	return fmt.Sprintf(`
INSERT INTO directories (%s)
SELECT %s
FROM directories_legacy;
`, strings.Join(columns, ", "), strings.Join(values, ", "))
}

func (c *Catalog) copyFilesSQL(state schemaState) string {
	columns := []string{"id", "directory_id", "basename", "extension", "size_bytes", "is_deleted"}
	values := []string{
		"id",
		"directory_id",
		"basename",
		"extension",
		c.columnExpression("size_bytes", state.fileColumns, "NULL"),
		c.columnExpression("is_deleted", state.fileColumns, "0"),
	}
	if c.options.IncludeTimestamps {
		createdAt := c.timestampValueExpression("created_at", state.fileColumns, `unixepoch()`)
		columns = append(columns, "created_at", "updated_at")
		values = append(values,
			createdAt,
			c.timestampValueExpression("updated_at", state.fileColumns, createdAt),
		)
	}

	return fmt.Sprintf(`
INSERT INTO files (%s)
SELECT %s
FROM files_legacy;
`, strings.Join(columns, ", "), strings.Join(values, ", "))
}

func (c *Catalog) copyMetadataSQL(state schemaState) string {
	columns := []string{"file_id", "external_link", "notes", "title_override"}
	values := []string{
		"file_id",
		c.columnExpression("external_link", state.metadataColumns, "NULL"),
		c.columnExpression("notes", state.metadataColumns, "NULL"),
		c.columnExpression("title_override", state.metadataColumns, "NULL"),
	}
	if c.options.IncludeTimestamps {
		createdAt := c.timestampValueExpression("created_at", state.metadataColumns, c.timestampValueExpression("updated_at", state.metadataColumns, `unixepoch()`))
		columns = append(columns, "created_at", "updated_at")
		values = append(values,
			createdAt,
			c.timestampValueExpression("updated_at", state.metadataColumns, createdAt),
		)
	}

	return fmt.Sprintf(`
INSERT INTO file_metadata (%s)
SELECT %s
FROM file_metadata_legacy;
`, strings.Join(columns, ", "), strings.Join(values, ", "))
}

func (c *Catalog) schemaSQL() string {
	parts := []string{
		c.directoriesTableSQL(),
		c.filesTableSQL(),
		c.fileMetadataTableSQL(),
		fmt.Sprintf(`PRAGMA user_version = %d;`, c.targetSchemaVersion()),
	}

	return strings.Join(parts, "\n\n")
}

func (c *Catalog) directoriesTableSQL() string {
	timestampColumns := ""
	if c.options.IncludeTimestamps {
		timestampColumns = ",\n  created_at INTEGER NOT NULL DEFAULT (unixepoch()),\n  updated_at INTEGER NOT NULL DEFAULT (unixepoch())"
	}

	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS directories (
  id INTEGER PRIMARY KEY,
  parent_id INTEGER REFERENCES directories(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  rel_path TEXT NOT NULL%s,
  UNIQUE(rel_path, name)
);`, timestampColumns)
}

func (c *Catalog) filesTableSQL() string {
	timestampColumns := ""
	if c.options.IncludeTimestamps {
		timestampColumns = ",\n  created_at INTEGER NOT NULL DEFAULT (unixepoch()),\n  updated_at INTEGER NOT NULL DEFAULT (unixepoch())"
	}

	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS files (
  id INTEGER PRIMARY KEY,
  directory_id INTEGER NOT NULL REFERENCES directories(id) ON DELETE CASCADE,
  basename TEXT NOT NULL,
  extension TEXT NOT NULL,
  size_bytes INTEGER,
  is_deleted INTEGER NOT NULL DEFAULT 0%s,
  UNIQUE(directory_id, basename)
);`, timestampColumns)
}

func (c *Catalog) fileMetadataTableSQL() string {
	timestampColumns := ""
	if c.options.IncludeTimestamps {
		timestampColumns = ",\n  created_at INTEGER NOT NULL DEFAULT (unixepoch()),\n  updated_at INTEGER NOT NULL DEFAULT (unixepoch())"
	}

	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS file_metadata (
  file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  external_link TEXT,
  notes TEXT,
  title_override TEXT%s,
  PRIMARY KEY(file_id)
) WITHOUT ROWID;`, timestampColumns)
}

func (c *Catalog) targetSchemaVersion() int {
	if c.options.IncludeTimestamps {
		return catalogSchemaVersionWithTimestamps
	}

	return catalogSchemaVersion
}

func (c *Catalog) userVersion() (int, error) {
	var version int
	err := c.db.QueryRow(`PRAGMA user_version`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("read sqlite schema version: %w", err)
	}

	return version, nil
}

func (c *Catalog) tableExists(name string) (bool, error) {
	var count int
	err := c.db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check sqlite table %q: %w", name, err)
	}

	return count > 0, nil
}

func (c *Catalog) tableColumns(tableName string) (map[string]columnInfo, error) {
	rows, err := c.db.Query(fmt.Sprintf(`PRAGMA table_info(%q)`, tableName))
	if err != nil {
		return nil, fmt.Errorf("inspect sqlite columns for %q: %w", tableName, err)
	}
	defer rows.Close()

	columns := make(map[string]columnInfo)
	for rows.Next() {
		var info columnInfo
		var cid int
		if err := rows.Scan(&cid, &info.name, &info.dataType, &info.notNull, &info.defaultExpr, &info.primaryKey); err != nil {
			return nil, fmt.Errorf("scan sqlite column for %q: %w", tableName, err)
		}
		columns[info.name] = info
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite columns for %q: %w", tableName, err)
	}

	return columns, nil
}

func (c *Catalog) hasColumn(tableName, columnName string) (bool, error) {
	columns, err := c.tableColumns(tableName)
	if err != nil {
		return false, err
	}
	_, ok := columns[columnName]
	return ok, nil
}

func (c *Catalog) tableSQLContains(tableName, token string) (bool, error) {
	var tableSQL sql.NullString
	err := c.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&tableSQL)
	if err != nil {
		return false, fmt.Errorf("read sqlite table SQL for %q: %w", tableName, err)
	}

	return strings.Contains(strings.ToUpper(tableSQL.String), strings.ToUpper(token)), nil
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

func (c *Catalog) dropRedundantIndexes() error {
	return c.dropRedundantIndexesWithTarget(c.db)
}

func (c *Catalog) dropRedundantIndexesWithTarget(target sqlExecTarget) error {
	for _, indexName := range []string{
		"idx_directories_parent_name",
		"idx_files_directory_basename",
		"idx_files_rel_path",
		"idx_files_hash",
		"idx_files_basename",
		"idx_files_directory_id",
		"idx_files_extension",
	} {
		if _, err := target.Exec(fmt.Sprintf(`DROP INDEX IF EXISTS %s`, indexName)); err != nil {
			return fmt.Errorf("drop redundant sqlite index %q: %w", indexName, err)
		}
	}

	return nil
}

func (c *Catalog) columnExpression(columnName string, columns map[string]columnInfo, fallback string) string {
	if _, ok := columns[columnName]; !ok {
		return fallback
	}

	return columnName
}

func (c *Catalog) timestampValueExpression(columnName string, columns map[string]columnInfo, fallback string) string {
	info, ok := columns[columnName]
	if !ok {
		return fallback
	}
	if strings.EqualFold(strings.TrimSpace(info.dataType), "INTEGER") {
		return columnName
	}

	return fmt.Sprintf(`COALESCE(unixepoch(%s), %s)`, columnName, fallback)
}

func columnsHaveAnyTimestamps(columns map[string]columnInfo) bool {
	if columns == nil {
		return false
	}
	_, hasCreated := columns["created_at"]
	_, hasUpdated := columns["updated_at"]
	return hasCreated || hasUpdated
}

func columnsHaveIntegerTimestamps(columns map[string]columnInfo) bool {
	if columns == nil {
		return false
	}
	createdAt, hasCreated := columns["created_at"]
	updatedAt, hasUpdated := columns["updated_at"]
	if !hasCreated || !hasUpdated {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(createdAt.dataType), "INTEGER") &&
		strings.EqualFold(strings.TrimSpace(updatedAt.dataType), "INTEGER")
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

	if _, err := c.execTarget().Exec(c.directoryUpsertSQL(), parentID, name, storedRelPath); err != nil {
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

func (c *Catalog) directoryUpsertSQL() string {
	updatedAtClause := ""
	if c.options.IncludeTimestamps {
		updatedAtClause = ",\n  updated_at = unixepoch()"
	}

	return fmt.Sprintf(`
INSERT INTO directories (parent_id, name, rel_path)
VALUES (?, ?, ?)
ON CONFLICT(rel_path, name) DO UPDATE SET
  parent_id = excluded.parent_id,
  name = excluded.name%s
`, updatedAtClause)
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

		if _, err := execTarget.Exec(c.fileUpsertSQL(), directoryID, file.Name, filepath.Ext(file.Name), info.Size()); err != nil {
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

func (c *Catalog) fileUpsertSQL() string {
	updatedAtClause := ""
	if c.options.IncludeTimestamps {
		updatedAtClause = ",\n  updated_at = unixepoch()"
	}

	return fmt.Sprintf(`
INSERT INTO files (directory_id, basename, extension, size_bytes, is_deleted)
VALUES (?, ?, ?, ?, 0)
ON CONFLICT(directory_id, basename) DO UPDATE SET
  basename = excluded.basename,
  extension = excluded.extension,
  size_bytes = excluded.size_bytes,
  is_deleted = 0%s
		`, updatedAtClause)
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
SELECT parent_id, name, rel_path
FROM directories
WHERE parent_id IS NOT NULL
ORDER BY rel_path COLLATE NOCASE, name COLLATE NOCASE
`)
	if err != nil {
		return nil, fmt.Errorf("read sqlite directories for search: %w", err)
	}
	defer directoryRows.Close()

	for directoryRows.Next() {
		var parentID sql.NullInt64
		var name, relPath string
		if err := directoryRows.Scan(&parentID, &name, &relPath); err != nil {
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
SELECT d.parent_id, d.rel_path, d.name, f.basename
FROM files AS f
JOIN directories AS d ON d.id = f.directory_id
WHERE f.is_deleted = 0
ORDER BY d.rel_path COLLATE NOCASE, d.name COLLATE NOCASE, f.basename COLLATE NOCASE
`)
	if err != nil {
		return nil, fmt.Errorf("read sqlite files for search: %w", err)
	}
	defer fileRows.Close()

	for fileRows.Next() {
		var parentID sql.NullInt64
		var relPath, directoryName, basename string
		if err := fileRows.Scan(&parentID, &relPath, &directoryName, &basename); err != nil {
			return nil, fmt.Errorf("scan sqlite search file: %w", err)
		}
		directoryRelPath := directoryActualRelPath(parentID.Valid, relPath, directoryName)
		if c.IsCatalogFile(joinRelativePath(directoryRelPath, basename)) {
			continue
		}
		entries = append(entries, SearchEntry{
			Name: basename,
			Type: "f",
			Path: displayPath(directoryRelPath),
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
	rows, err := c.query(`
SELECT f.id, d.parent_id, d.rel_path, d.name, f.basename
FROM files AS f
JOIN directories AS d ON d.id = f.directory_id
`)
	if err != nil {
		return nil, fmt.Errorf("read sqlite files for pruning: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		var parentID sql.NullInt64
		var relPath, directoryName, basename string
		if err := rows.Scan(&id, &parentID, &relPath, &directoryName, &basename); err != nil {
			return nil, fmt.Errorf("scan sqlite file for pruning: %w", err)
		}

		directoryRelPath := directoryActualRelPath(parentID.Valid, relPath, directoryName)
		actualRelPath := joinRelativePath(directoryRelPath, basename)
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
SELECT id, parent_id, rel_path, name
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
		var parentID sql.NullInt64
		var relPath, name string
		if err := rows.Scan(&id, &parentID, &relPath, &name); err != nil {
			return nil, fmt.Errorf("scan sqlite directory for pruning: %w", err)
		}

		actualRelPath := directoryActualRelPath(parentID.Valid, relPath, name)
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

func directoryActualRelPath(hasParent bool, relPath, name string) string {
	if !hasParent {
		return ""
	}
	return joinStoredRelativePath(relPath, name)
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
