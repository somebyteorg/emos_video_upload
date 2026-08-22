package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const databaseSchemaVersion = 2

type Database struct {
	sql *sql.DB
}

func OpenDatabase(path string) (*Database, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db := &Database{sql: conn}
	db.sql.SetMaxOpenConns(1)
	db.sql.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.configure(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := db.initializeSchema(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return db, nil
}

func (d *Database) configure(ctx context.Context) error {
	for _, statement := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = 10000`,
	} {
		if _, err := d.sql.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure sqlite database: %w", err)
		}
	}
	return nil
}

func (d *Database) initializeSchema(ctx context.Context) error {
	var version int
	if err := d.sql.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read sqlite schema version: %w", err)
	}
	if version == databaseSchemaVersion {
		return nil
	}

	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite schema setup: %w", err)
	}
	rollback := func(cause error) error {
		_ = tx.Rollback()
		return fmt.Errorf("initialize sqlite schema: %w", cause)
	}
	for _, table := range []string{"tasks", "upload_tokens", "probe_cache", "directory_metadata"} {
		if _, err = tx.ExecContext(ctx, `DROP TABLE IF EXISTS `+table); err != nil {
			return rollback(err)
		}
	}
	_, err = tx.ExecContext(ctx, `
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			path TEXT NOT NULL,
			file_name TEXT NOT NULL,
			file_size INTEGER NOT NULL CHECK (file_size > 0),
			file_mtime_ns INTEGER NOT NULL,
			item_type TEXT NOT NULL CHECK (item_type IN ('vl', 've')),
			item_id INTEGER NOT NULL CHECK (item_id > 0),
			season_number INTEGER NOT NULL DEFAULT 0,
			episode_number INTEGER NOT NULL DEFAULT 0,
			video_title TEXT NOT NULL DEFAULT '',
			storage TEXT NOT NULL DEFAULT '',
			generate_sprites INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'success', 'error')),
			stage TEXT NOT NULL DEFAULT '',
			progress REAL NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 100),
			uploaded_bytes INTEGER NOT NULL DEFAULT 0,
			total_bytes INTEGER NOT NULL DEFAULT 0,
			file_id TEXT NOT NULL DEFAULT '',
			media_id TEXT NOT NULL DEFAULT '',
			probe_metadata TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE INDEX idx_tasks_created_at ON tasks(created_at DESC);
		CREATE INDEX idx_tasks_status ON tasks(status);
		CREATE INDEX idx_tasks_path ON tasks(path);
		CREATE UNIQUE INDEX idx_tasks_active_identity ON tasks(
			path, file_size, file_mtime_ns, kind, item_type, item_id,
			season_number, episode_number, storage
		) WHERE status IN ('queued', 'running');

		CREATE TABLE upload_tokens (
			cache_key TEXT PRIMARY KEY,
			path TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			file_mtime_ns INTEGER NOT NULL,
			storage TEXT NOT NULL,
			token_type TEXT NOT NULL,
			file_id TEXT NOT NULL,
			upload_url TEXT NOT NULL DEFAULT '',
			raw_json TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			completed INTEGER NOT NULL DEFAULT 0,
			presigns_json TEXT NOT NULL DEFAULT '',
			multipart_completed INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX idx_upload_tokens_expiry ON upload_tokens(expires_at);

		CREATE TABLE probe_cache (
			path TEXT PRIMARY KEY,
			file_size INTEGER NOT NULL,
			file_mtime_ns INTEGER NOT NULL,
			metadata TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE directory_metadata (
			path TEXT PRIMARY KEY,
			todb_id INTEGER NOT NULL CHECK (todb_id > 0),
			updated_at TEXT NOT NULL
		);

		PRAGMA user_version = 2;
	`)
	if err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite schema setup: %w", err)
	}
	return nil
}

func (d *Database) Close() error {
	return d.sql.Close()
}

type TaskRecord struct {
	ID              string
	Kind            string
	Path            string
	FileName        string
	FileSize        int64
	FileMtimeNS     int64
	ItemType        string
	ItemID          int64
	SeasonNumber    int
	EpisodeNumber   int
	VideoTitle      string
	Storage         string
	GenerateSprites bool
	Status          string
	Stage           string
	Progress        float64
	UploadedBytes   int64
	TotalBytes      int64
	FileID          string
	MediaID         string
	ProbeMetadata   string
	Error           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

const taskColumns = `
	id, kind, path, file_name, file_size, file_mtime_ns, item_type, item_id,
	season_number, episode_number, video_title, storage, generate_sprites,
	status, stage, progress, uploaded_bytes, total_bytes, file_id, media_id,
	probe_metadata, error, created_at, updated_at
`

func (d *Database) GetTask(ctx context.Context, id string) (TaskRecord, error) {
	return scanTask(d.sql.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id))
}

func (d *Database) CreateTask(ctx context.Context, task TaskRecord) (TaskRecord, bool, error) {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return TaskRecord{}, false, err
	}
	rollback := func(cause error) (TaskRecord, bool, error) {
		_ = tx.Rollback()
		return TaskRecord{}, false, cause
	}

	existing, findErr := scanTask(tx.QueryRowContext(ctx, `
		SELECT `+taskColumns+` FROM tasks
		WHERE path = ? AND file_size = ? AND file_mtime_ns = ?
			AND kind = ? AND item_type = ? AND item_id = ?
			AND season_number = ? AND episode_number = ? AND storage = ?
			AND status IN ('queued', 'running')
		ORDER BY created_at DESC LIMIT 1
	`, task.Path, task.FileSize, task.FileMtimeNS, task.Kind, task.ItemType, task.ItemID,
		task.SeasonNumber, task.EpisodeNumber, task.Storage))
	switch {
	case findErr == nil:
		if err := tx.Commit(); err != nil {
			return TaskRecord{}, false, fmt.Errorf("commit existing task lookup: %w", err)
		}
		return existing, false, nil
	case !errors.Is(findErr, sql.ErrNoRows):
		return rollback(findErr)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO tasks (
			id, kind, path, file_name, file_size, file_mtime_ns, item_type, item_id,
			season_number, episode_number, video_title, storage, generate_sprites,
			status, stage, progress, uploaded_bytes, total_bytes, file_id, media_id,
			probe_metadata, error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.Kind, task.Path, task.FileName, task.FileSize, task.FileMtimeNS,
		task.ItemType, task.ItemID, task.SeasonNumber, task.EpisodeNumber, task.VideoTitle,
		task.Storage, boolInt(task.GenerateSprites), task.Status, task.Stage, task.Progress,
		task.UploadedBytes, task.TotalBytes, task.FileID, task.MediaID, task.ProbeMetadata,
		task.Error, formatTime(task.CreatedAt), formatTime(task.UpdatedAt))
	if err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return TaskRecord{}, false, fmt.Errorf("commit task creation: %w", err)
	}
	return task, true, nil
}

func (d *Database) ListTasksPage(ctx context.Context, limit, offset int) ([]TaskRecord, int, error) {
	limit = clampPageLimit(limit)
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := d.sql.QueryContext(ctx, `
		SELECT `+taskColumns+` FROM tasks
		ORDER BY created_at DESC LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	tasks, err := scanTasks(rows)
	return tasks, total, err
}

func (d *Database) ListResumableTasks(ctx context.Context) ([]TaskRecord, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT `+taskColumns+` FROM tasks
		WHERE status IN ('queued', 'running')
		ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (d *Database) ResetRunningTasks(ctx context.Context) error {
	_, err := d.sql.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'queued', stage = '等待恢复', error = '', updated_at = ?
		WHERE status = 'running'
	`, formatTime(time.Now()))
	return err
}

func (d *Database) ResetTaskForRetry(ctx context.Context, id string) error {
	result, err := d.sql.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'queued', stage = '等待重试', error = '', updated_at = ?
		WHERE id = ? AND status <> 'success'
	`, formatTime(time.Now()), id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *Database) UpdateTask(ctx context.Context, task TaskRecord) error {
	result, err := d.sql.ExecContext(ctx, `
		UPDATE tasks SET
			video_title = ?, status = ?, stage = ?, progress = ?,
			uploaded_bytes = ?, total_bytes = ?, file_id = ?, media_id = ?,
			probe_metadata = ?, error = ?, updated_at = ?
		WHERE id = ?
	`, task.VideoTitle, task.Status, task.Stage, clampFloat(task.Progress, 0, 100),
		task.UploadedBytes, task.TotalBytes, task.FileID, task.MediaID, task.ProbeMetadata,
		task.Error, formatTime(time.Now()), task.ID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *Database) DeleteTask(ctx context.Context, id string) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM upload_tokens
		WHERE completed = 0
			AND NOT EXISTS (
				SELECT 1 FROM tasks
				WHERE tasks.path = upload_tokens.path
					AND tasks.file_size = upload_tokens.file_size
					AND tasks.file_mtime_ns = upload_tokens.file_mtime_ns
					AND tasks.storage = upload_tokens.storage
					AND tasks.status IN ('queued', 'running')
			)
	`); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (d *Database) DeleteCompletedTasks(ctx context.Context) (int, error) {
	result, err := d.sql.ExecContext(ctx, `DELETE FROM tasks WHERE status = 'success'`)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	return int(count), err
}

func (d *Database) FindProbe(ctx context.Context, path string, size, mtime int64) (string, error) {
	var metadata string
	err := d.sql.QueryRowContext(ctx, `
		SELECT metadata FROM probe_cache
		WHERE path = ? AND file_size = ? AND file_mtime_ns = ?
	`, path, size, mtime).Scan(&metadata)
	return metadata, err
}

func (d *Database) SaveProbe(ctx context.Context, path string, size, mtime int64, metadata string) error {
	now := formatTime(time.Now())
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO probe_cache(path, file_size, file_mtime_ns, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			file_size = excluded.file_size,
			file_mtime_ns = excluded.file_mtime_ns,
			metadata = excluded.metadata,
			updated_at = excluded.updated_at
	`, path, size, mtime, metadata, now, now)
	return err
}

func (d *Database) GetDirectoryTODBID(ctx context.Context, path string) (int64, error) {
	var id int64
	err := d.sql.QueryRowContext(ctx, `SELECT todb_id FROM directory_metadata WHERE path = ?`, path).Scan(&id)
	return id, err
}

func (d *Database) SaveDirectoryTODBID(ctx context.Context, path string, id int64) error {
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO directory_metadata(path, todb_id, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			todb_id = excluded.todb_id,
			updated_at = excluded.updated_at
	`, path, id, formatTime(time.Now()))
	return err
}

type UploadTokenRecord struct {
	CacheKey           string
	Path               string
	FileSize           int64
	FileMtimeNS        int64
	Storage            string
	TokenType          string
	FileID             string
	UploadURL          string
	RawJSON            string
	ExpiresAt          time.Time
	Completed          bool
	PresignsJSON       string
	MultipartCompleted bool
}

func (d *Database) GetUploadToken(ctx context.Context, cacheKey string) (UploadTokenRecord, error) {
	return scanUploadToken(d.sql.QueryRowContext(ctx, `
		SELECT cache_key, path, file_size, file_mtime_ns, storage, token_type,
			file_id, upload_url, raw_json, expires_at, completed,
			presigns_json, multipart_completed
		FROM upload_tokens WHERE cache_key = ?
	`, cacheKey))
}

func (d *Database) SaveUploadToken(ctx context.Context, record UploadTokenRecord) error {
	now := formatTime(time.Now())
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO upload_tokens (
			cache_key, path, file_size, file_mtime_ns, storage, token_type, file_id,
			upload_url, raw_json, expires_at, completed, presigns_json,
			multipart_completed, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET
			path = excluded.path,
			file_size = excluded.file_size,
			file_mtime_ns = excluded.file_mtime_ns,
			storage = excluded.storage,
			token_type = excluded.token_type,
			file_id = excluded.file_id,
			upload_url = excluded.upload_url,
			raw_json = excluded.raw_json,
			expires_at = excluded.expires_at,
			completed = excluded.completed,
			presigns_json = excluded.presigns_json,
			multipart_completed = excluded.multipart_completed,
			updated_at = excluded.updated_at
	`, record.CacheKey, record.Path, record.FileSize, record.FileMtimeNS, record.Storage,
		record.TokenType, record.FileID, record.UploadURL, record.RawJSON,
		formatTime(record.ExpiresAt), boolInt(record.Completed), record.PresignsJSON,
		boolInt(record.MultipartCompleted), now, now)
	return err
}

func (d *Database) SaveMultipartPresigns(ctx context.Context, cacheKey, value string) error {
	_, err := d.sql.ExecContext(ctx, `
		UPDATE upload_tokens
		SET presigns_json = ?, updated_at = ?
		WHERE cache_key = ?
	`, value, formatTime(time.Now()), cacheKey)
	return err
}

func (d *Database) MarkMultipartCompleted(ctx context.Context, cacheKey string) error {
	_, err := d.sql.ExecContext(ctx, `
		UPDATE upload_tokens SET multipart_completed = 1, updated_at = ?
		WHERE cache_key = ?
	`, formatTime(time.Now()), cacheKey)
	return err
}

func (d *Database) CompleteUploadToken(ctx context.Context, cacheKey string) error {
	_, err := d.sql.ExecContext(ctx, `
		UPDATE upload_tokens SET completed = 1, updated_at = ?
		WHERE cache_key = ?
	`, formatTime(time.Now()), cacheKey)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTasks(rows *sql.Rows) ([]TaskRecord, error) {
	result := make([]TaskRecord, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, task)
	}
	return result, rows.Err()
}

func scanTask(row scanner) (TaskRecord, error) {
	var task TaskRecord
	var generateSprites int
	var createdAt, updatedAt string
	if err := row.Scan(
		&task.ID, &task.Kind, &task.Path, &task.FileName, &task.FileSize, &task.FileMtimeNS,
		&task.ItemType, &task.ItemID, &task.SeasonNumber, &task.EpisodeNumber,
		&task.VideoTitle, &task.Storage, &generateSprites, &task.Status, &task.Stage,
		&task.Progress, &task.UploadedBytes, &task.TotalBytes, &task.FileID,
		&task.MediaID, &task.ProbeMetadata, &task.Error, &createdAt, &updatedAt,
	); err != nil {
		return TaskRecord{}, err
	}
	task.GenerateSprites = generateSprites != 0
	task.CreatedAt = parseTime(createdAt)
	task.UpdatedAt = parseTime(updatedAt)
	return task, nil
}

func scanUploadToken(row scanner) (UploadTokenRecord, error) {
	var record UploadTokenRecord
	var expiresAt string
	var completed int
	var multipartCompleted int
	if err := row.Scan(
		&record.CacheKey, &record.Path, &record.FileSize, &record.FileMtimeNS,
		&record.Storage, &record.TokenType, &record.FileID, &record.UploadURL,
		&record.RawJSON, &expiresAt, &completed, &record.PresignsJSON,
		&multipartCompleted,
	); err != nil {
		return UploadTokenRecord{}, err
	}
	record.ExpiresAt = parseTime(expiresAt)
	record.Completed = completed != 0
	record.MultipartCompleted = multipartCompleted != 0
	return record, nil
}

func parseTime(value string) time.Time {
	result, _ := time.Parse(time.RFC3339Nano, value)
	return result
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func encodeJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func clampPageLimit(limit int) int {
	if limit <= 0 || limit > 200 {
		return 20
	}
	return limit
}
