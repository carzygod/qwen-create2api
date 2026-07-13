package internal

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

var AppStore *Store

type AccountRecord struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	Status           string `json:"status"`
	Enabled          bool   `json:"enabled"`
	Priority         int    `json:"priority"`
	Weight           int    `json:"weight"`
	CookieJSON       string `json:"cookie_json,omitempty"`
	CookieString     string `json:"cookie_string,omitempty"`
	LocalStorageJSON string `json:"local_storage_json,omitempty"`
	XsrfToken        string `json:"xsrf_token,omitempty"`
	DeviceID         string `json:"device_id,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	ProxyURL         string `json:"proxy_url,omitempty"`
	CapabilitiesJSON string `json:"capabilities_json,omitempty"`
	QuotaJSON        string `json:"quota_json,omitempty"`
	LastError        string `json:"last_error,omitempty"`
	LastTestAt       string `json:"last_test_at,omitempty"`
	LastSuccessAt    string `json:"last_success_at,omitempty"`
	LastQuotaSyncAt  string `json:"last_quota_sync_at,omitempty"`
	CooldownUntil    string `json:"cooldown_until,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type ModelRecord struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	DisplayName      string `json:"display_name"`
	UpstreamModel    string `json:"upstream_model"`
	Enabled          bool   `json:"enabled"`
	IsDefault        bool   `json:"is_default"`
	ParamsSchemaJSON string `json:"params_schema_json,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type TaskRecord struct {
	ID                     string `json:"id"`
	Type                   string `json:"type"`
	Status                 string `json:"status"`
	Model                  string `json:"model,omitempty"`
	ProviderAccountID      string `json:"provider_account_id,omitempty"`
	UpstreamTaskID         string `json:"upstream_task_id,omitempty"`
	UpstreamConversationID string `json:"upstream_conversation_id,omitempty"`
	RequestJSON            string `json:"request_json"`
	UpstreamRequestJSON    string `json:"upstream_request_json,omitempty"`
	UpstreamResponseJSON   string `json:"upstream_response_json,omitempty"`
	ResultJSON             string `json:"result_json,omitempty"`
	ErrorCode              string `json:"error_code,omitempty"`
	ErrorMessage           string `json:"error_message,omitempty"`
	RetryCount             int    `json:"retry_count"`
	MaxRetries             int    `json:"max_retries"`
	NextPollAt             string `json:"next_poll_at,omitempty"`
	StartedAt              string `json:"started_at,omitempty"`
	CompletedAt            string `json:"completed_at,omitempty"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
}

func InitStore() error {
	if err := os.MkdirAll(Cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(Cfg.DatabasePath), 0755); err != nil {
		return fmt.Errorf("create database dir: %w", err)
	}

	db, err := sql.Open("sqlite", Cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return err
	}
	if err := store.seedModels(); err != nil {
		_ = db.Close()
		return err
	}
	AppStore = store
	return nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`PRAGMA journal_mode=WAL;`,
		`CREATE TABLE IF NOT EXISTS qianwen_accounts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'unknown',
			enabled INTEGER NOT NULL DEFAULT 1,
			priority INTEGER NOT NULL DEFAULT 100,
			weight INTEGER NOT NULL DEFAULT 1,
			cookie_json TEXT,
			cookie_string TEXT,
			local_storage_json TEXT,
			xsrf_token TEXT,
			device_id TEXT,
			user_agent TEXT,
			proxy_url TEXT,
			capabilities_json TEXT,
			quota_json TEXT,
			last_error TEXT,
			last_test_at TEXT,
			last_success_at TEXT,
			last_quota_sync_at TEXT,
			cooldown_until TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS qianwen_account_events (
			id TEXT PRIMARY KEY,
			account_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			message TEXT,
			metadata_json TEXT,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS qianwen_account_maintenance (
			account_id TEXT PRIMARY KEY,
			state TEXT NOT NULL DEFAULT 'active',
			lease_owner TEXT,
			lease_expires_at TEXT,
			profile_path TEXT,
			last_error TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS qianwen_models (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			display_name TEXT,
			upstream_model TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			is_default INTEGER NOT NULL DEFAULT 0,
			params_schema_json TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS qianwen_tasks (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			model TEXT,
			provider_account_id TEXT,
			upstream_task_id TEXT,
			upstream_conversation_id TEXT,
			request_json TEXT NOT NULL,
			upstream_request_json TEXT,
			upstream_response_json TEXT,
			result_json TEXT,
			error_code TEXT,
			error_message TEXT,
			retry_count INTEGER NOT NULL DEFAULT 0,
			max_retries INTEGER NOT NULL DEFAULT 2,
			next_poll_at TEXT,
			started_at TEXT,
			completed_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS qianwen_task_events (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			message TEXT,
			metadata_json TEXT,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS qianwen_assets (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			type TEXT NOT NULL,
			upstream_url TEXT,
			local_path TEXT,
			public_url TEXT,
			mime_type TEXT,
			size_bytes INTEGER,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS qianwen_api_keys (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			key_hash TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS qianwen_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("sqlite migration failed: %w", err)
		}
	}
	return nil
}

func (s *Store) seedModels() error {
	now := nowISO()
	models := []ModelRecord{
		{ID: "qianwen-creator-session", Type: "chat", DisplayName: "Qianwen Creator Session Probe", UpstreamModel: "creator-session", Enabled: true, IsDefault: Cfg.DefaultChatModel == "qianwen-creator-session", CreatedAt: now, UpdatedAt: now},
		{ID: "qianwen-creator-wan21-frame", Type: "video", DisplayName: "Wan 2.1 First/Last Frame", UpstreamModel: "wanx2.1-kf2v-plus", Enabled: true, IsDefault: Cfg.DefaultVideoModel == "qianwen-creator-wan21-frame", ParamsSchemaJSON: `{"duration":true,"aspect_ratio":true,"resolution":true,"first_frame_image":true,"last_frame_image":true,"first_frame_material_id":true,"last_frame_material_id":true}`, CreatedAt: now, UpdatedAt: now},
		{ID: "qianwen-creator-wan22-flash-frame", Type: "video", DisplayName: "Wan 2.2 Flash First/Last Frame", UpstreamModel: "wan2.2-kf2v-flash", Enabled: true, IsDefault: Cfg.DefaultVideoModel == "qianwen-creator-wan22-flash-frame", ParamsSchemaJSON: `{"duration":true,"aspect_ratio":true,"resolution":true,"first_frame_image":true,"last_frame_image":true,"first_frame_material_id":true,"last_frame_material_id":true}`, CreatedAt: now, UpdatedAt: now},
		{ID: "qianwen-creator-wan25-i2v", Type: "video", DisplayName: "Wan 2.5 First Frame Video", UpstreamModel: "wan2.5-i2v-preview", Enabled: true, IsDefault: Cfg.DefaultVideoModel == "qianwen-creator-wan25-i2v", ParamsSchemaJSON: `{"duration":true,"aspect_ratio":true,"resolution":true,"first_frame_image":true,"first_frame_material_id":true}`, CreatedAt: now, UpdatedAt: now},
		{ID: "qianwen-creator-wan25-t2v", Type: "video", DisplayName: "Wan 2.5 Text Video", UpstreamModel: "wan2.5-t2v-preview", Enabled: true, IsDefault: Cfg.DefaultVideoModel == "qianwen-creator-wan25-t2v", ParamsSchemaJSON: `{"duration":true,"aspect_ratio":true,"resolution":true}`, CreatedAt: now, UpdatedAt: now},
		{ID: "qianwen-creator-wan27-frame", Type: "video", DisplayName: "Wan 2.7 First/Last Frame", UpstreamModel: "wan2.7-i2v", Enabled: true, IsDefault: Cfg.DefaultVideoModel == "qianwen-creator-wan27-frame", ParamsSchemaJSON: `{"duration":true,"aspect_ratio":true,"resolution":true,"first_frame_image":true,"last_frame_image":true,"first_frame_material_id":true,"last_frame_material_id":true}`, CreatedAt: now, UpdatedAt: now},
		{ID: "qianwen-creator-happyhorse-i2v", Type: "video", DisplayName: "HappyHorse First Frame Video", UpstreamModel: "happyhorse", Enabled: true, IsDefault: Cfg.DefaultVideoModel == "qianwen-creator-happyhorse-i2v", ParamsSchemaJSON: `{"duration":true,"aspect_ratio":true,"resolution":true,"first_frame_image":true,"first_frame_material_id":true}`, CreatedAt: now, UpdatedAt: now},
	}
	for _, m := range models {
		_, err := s.db.Exec(`INSERT INTO qianwen_models
			(id, type, display_name, upstream_model, enabled, is_default, params_schema_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				type=excluded.type,
				display_name=excluded.display_name,
				upstream_model=excluded.upstream_model,
				enabled=excluded.enabled,
				is_default=CASE WHEN qianwen_models.is_default=1 THEN 1 ELSE excluded.is_default END,
				params_schema_json=excluded.params_schema_json,
				updated_at=excluded.updated_at`,
			m.ID, m.Type, m.DisplayName, m.UpstreamModel, boolToInt(m.Enabled), boolToInt(m.IsDefault), m.ParamsSchemaJSON, m.CreatedAt, m.UpdatedAt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListModels() ([]ModelRecord, error) {
	rows, err := s.db.Query(`SELECT id, type, display_name, upstream_model, enabled, is_default, COALESCE(params_schema_json,''), created_at, updated_at FROM qianwen_models WHERE enabled=1 ORDER BY type, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var models []ModelRecord
	for rows.Next() {
		var m ModelRecord
		var enabled, isDefault int
		if err := rows.Scan(&m.ID, &m.Type, &m.DisplayName, &m.UpstreamModel, &enabled, &isDefault, &m.ParamsSchemaJSON, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		m.Enabled = enabled == 1
		m.IsDefault = isDefault == 1
		models = append(models, m)
	}
	return models, rows.Err()
}

func (s *Store) CreateAccount(a *AccountRecord) error {
	now := nowISO()
	if strings.TrimSpace(a.ID) == "" {
		a.ID = uuid.New().String()
	}
	a.Status = defaultString(a.Status, "unknown")
	a.Type = defaultString(a.Type, "login_cookie")
	a.CreatedAt = now
	a.UpdatedAt = now
	if a.Priority == 0 {
		a.Priority = 100
	}
	if a.Weight == 0 {
		a.Weight = 1
	}
	if a.Name == "" {
		return errors.New("account name is required")
	}
	if a.CapabilitiesJSON == "" {
		a.CapabilitiesJSON = defaultCapabilitiesForType(a.Type)
	}
	_, err := s.db.Exec(`INSERT INTO qianwen_accounts
		(id, name, type, status, enabled, priority, weight, cookie_json, cookie_string, local_storage_json,
		 xsrf_token, device_id, user_agent, proxy_url, capabilities_json, quota_json, last_error,
		 last_test_at, last_success_at, last_quota_sync_at, cooldown_until, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Type, a.Status, boolToInt(a.Enabled), a.Priority, a.Weight, a.CookieJSON, a.CookieString, a.LocalStorageJSON,
		a.XsrfToken, a.DeviceID, a.UserAgent, a.ProxyURL, a.CapabilitiesJSON, a.QuotaJSON, a.LastError,
		a.LastTestAt, a.LastSuccessAt, a.LastQuotaSyncAt, a.CooldownUntil, a.CreatedAt, a.UpdatedAt)
	return err
}

func (s *Store) ListAccounts() ([]AccountRecord, error) {
	rows, err := s.db.Query(`SELECT id, name, type, status, enabled, priority, weight,
		COALESCE(cookie_json,''), COALESCE(cookie_string,''), COALESCE(local_storage_json,''), COALESCE(xsrf_token,''), COALESCE(device_id,''),
		COALESCE(user_agent,''), COALESCE(proxy_url,''), COALESCE(capabilities_json,''), COALESCE(quota_json,''), COALESCE(last_error,''),
		COALESCE(last_test_at,''), COALESCE(last_success_at,''), COALESCE(last_quota_sync_at,''), COALESCE(cooldown_until,''),
		created_at, updated_at
		FROM qianwen_accounts ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []AccountRecord
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *Store) GetAccount(id string) (*AccountRecord, error) {
	row := s.db.QueryRow(`SELECT id, name, type, status, enabled, priority, weight,
		COALESCE(cookie_json,''), COALESCE(cookie_string,''), COALESCE(local_storage_json,''), COALESCE(xsrf_token,''), COALESCE(device_id,''),
		COALESCE(user_agent,''), COALESCE(proxy_url,''), COALESCE(capabilities_json,''), COALESCE(quota_json,''), COALESCE(last_error,''),
		COALESCE(last_test_at,''), COALESCE(last_success_at,''), COALESCE(last_quota_sync_at,''), COALESCE(cooldown_until,''),
		created_at, updated_at
		FROM qianwen_accounts WHERE id=?`, id)
	a, err := scanAccount(row)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) DeleteAccount(id string) error {
	if active, err := s.IsAccountInMaintenance(id); err != nil {
		return err
	} else if active {
		return errors.New("account has an active maintenance lease")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM qianwen_account_maintenance WHERE account_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM qianwen_accounts WHERE id=?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_ = removeAccountProfile(id)
	return nil
}

func (s *Store) UpdateAccountStatus(id, status, lastError string, success bool) error {
	now := nowISO()
	lastSuccess := ""
	if success {
		lastSuccess = now
	}
	if success {
		_, err := s.db.Exec(`UPDATE qianwen_accounts SET status=?, last_error=?, last_test_at=?, last_success_at=?, updated_at=? WHERE id=?`,
			status, lastError, now, lastSuccess, now, id)
		return err
	}
	_, err := s.db.Exec(`UPDATE qianwen_accounts SET status=?, last_error=?, last_test_at=?, updated_at=? WHERE id=?`,
		status, lastError, now, now, id)
	return err
}

func (s *Store) SelectAccountForCapability(capability string) (*AccountRecord, error) {
	accounts, err := s.ListAccounts()
	if err != nil {
		return nil, err
	}
	for _, a := range accounts {
		if !a.Enabled || a.Status != "valid" {
			continue
		}
		inMaintenance, err := s.IsAccountInMaintenance(a.ID)
		if err != nil {
			return nil, err
		}
		if inMaintenance {
			continue
		}
		if accountSupportsCapability(a, capability) {
			return &a, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *Store) SelectRunnableAccountForCapability(capability string) (*AccountRecord, error) {
	accounts, err := s.ListRunnableAccountsForCapability(capability)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, sql.ErrNoRows
	}
	return &accounts[0], nil
}

func (s *Store) ListRunnableAccountsForCapability(capability string) ([]AccountRecord, error) {
	accounts, err := s.ListAccounts()
	if err != nil {
		return nil, err
	}
	var runnable []AccountRecord
	for _, a := range accounts {
		if !a.Enabled || a.Status != "valid" {
			continue
		}
		inMaintenance, err := s.IsAccountInMaintenance(a.ID)
		if err != nil {
			return nil, err
		}
		if inMaintenance {
			continue
		}
		if strings.TrimSpace(a.CookieJSON) == "" && strings.TrimSpace(a.CookieString) == "" {
			continue
		}
		if accountSupportsCapability(a, capability) {
			runnable = append(runnable, a)
		}
	}
	return runnable, nil
}

func (s *Store) CreateTask(t *TaskRecord) error {
	now := nowISO()
	t.ID = uuid.New().String()
	t.Status = defaultString(t.Status, "queued")
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.MaxRetries == 0 {
		t.MaxRetries = 2
	}
	_, err := s.db.Exec(`INSERT INTO qianwen_tasks
		(id, type, status, model, provider_account_id, upstream_task_id, upstream_conversation_id, request_json,
		 upstream_request_json, upstream_response_json, result_json, error_code, error_message, retry_count, max_retries,
		 next_poll_at, started_at, completed_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Type, t.Status, t.Model, t.ProviderAccountID, t.UpstreamTaskID, t.UpstreamConversationID, t.RequestJSON,
		t.UpstreamRequestJSON, t.UpstreamResponseJSON, t.ResultJSON, t.ErrorCode, t.ErrorMessage, t.RetryCount, t.MaxRetries,
		t.NextPollAt, t.StartedAt, t.CompletedAt, t.CreatedAt, t.UpdatedAt)
	return err
}

func (s *Store) ListTasks(limit int) ([]TaskRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, type, status, COALESCE(model,''), COALESCE(provider_account_id,''), COALESCE(upstream_task_id,''),
		COALESCE(upstream_conversation_id,''), request_json, COALESCE(upstream_request_json,''), COALESCE(upstream_response_json,''),
		COALESCE(result_json,''), COALESCE(error_code,''), COALESCE(error_message,''), retry_count, max_retries,
		COALESCE(next_poll_at,''), COALESCE(started_at,''), COALESCE(completed_at,''), created_at, updated_at
		FROM qianwen_tasks ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []TaskRecord
	for rows.Next() {
		var t TaskRecord
		if err := rows.Scan(&t.ID, &t.Type, &t.Status, &t.Model, &t.ProviderAccountID, &t.UpstreamTaskID,
			&t.UpstreamConversationID, &t.RequestJSON, &t.UpstreamRequestJSON, &t.UpstreamResponseJSON,
			&t.ResultJSON, &t.ErrorCode, &t.ErrorMessage, &t.RetryCount, &t.MaxRetries,
			&t.NextPollAt, &t.StartedAt, &t.CompletedAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (s *Store) GetTask(id string) (*TaskRecord, error) {
	row := s.db.QueryRow(`SELECT id, type, status, COALESCE(model,''), COALESCE(provider_account_id,''), COALESCE(upstream_task_id,''),
		COALESCE(upstream_conversation_id,''), request_json, COALESCE(upstream_request_json,''), COALESCE(upstream_response_json,''),
		COALESCE(result_json,''), COALESCE(error_code,''), COALESCE(error_message,''), retry_count, max_retries,
		COALESCE(next_poll_at,''), COALESCE(started_at,''), COALESCE(completed_at,''), created_at, updated_at
		FROM qianwen_tasks WHERE id=?`, id)
	var t TaskRecord
	if err := row.Scan(&t.ID, &t.Type, &t.Status, &t.Model, &t.ProviderAccountID, &t.UpstreamTaskID,
		&t.UpstreamConversationID, &t.RequestJSON, &t.UpstreamRequestJSON, &t.UpstreamResponseJSON,
		&t.ResultJSON, &t.ErrorCode, &t.ErrorMessage, &t.RetryCount, &t.MaxRetries,
		&t.NextPollAt, &t.StartedAt, &t.CompletedAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) UpdateTaskRunning(id, upstreamRequestJSON, upstreamResponseJSON string) error {
	now := nowISO()
	_, err := s.db.Exec(`UPDATE qianwen_tasks SET status='processing', upstream_request_json=?, upstream_response_json=?, started_at=COALESCE(NULLIF(started_at,''), ?), updated_at=? WHERE id=? AND status NOT IN ('cancelled','succeeded')`,
		upstreamRequestJSON, upstreamResponseJSON, now, now, id)
	return err
}

func (s *Store) UpdateTaskRunningWithAccount(id, accountID, upstreamTaskID, upstreamConversationID, upstreamRequestJSON, upstreamResponseJSON string) error {
	now := nowISO()
	_, err := s.db.Exec(`UPDATE qianwen_tasks
		SET status='processing',
			provider_account_id=?,
			upstream_task_id=?,
			upstream_conversation_id=?,
			upstream_request_json=?,
			upstream_response_json=?,
			started_at=COALESCE(NULLIF(started_at,''), ?),
			updated_at=?
		WHERE id=? AND status NOT IN ('cancelled','succeeded')`,
		accountID, upstreamTaskID, upstreamConversationID, upstreamRequestJSON, upstreamResponseJSON, now, now, id)
	return err
}

func (s *Store) UpdateTaskCompleted(id, resultJSON, upstreamResponseJSON string) error {
	now := nowISO()
	_, err := s.db.Exec(`UPDATE qianwen_tasks SET status='succeeded', result_json=?, upstream_response_json=?, error_code='', error_message='', completed_at=?, updated_at=? WHERE id=? AND status != 'cancelled'`,
		resultJSON, upstreamResponseJSON, now, now, id)
	return err
}

func (s *Store) UpdateTaskFailed(id, code, message, upstreamResponseJSON string) error {
	now := nowISO()
	_, err := s.db.Exec(`UPDATE qianwen_tasks SET status='failed', error_code=?, error_message=?, upstream_response_json=?, completed_at=?, updated_at=? WHERE id=? AND status != 'cancelled'`,
		code, message, upstreamResponseJSON, now, now, id)
	return err
}

func (s *Store) CancelTask(id string) error {
	now := nowISO()
	result, err := s.db.Exec(`UPDATE qianwen_tasks
		SET status='cancelled',
			error_code='cancelled',
			error_message='Task was cancelled locally.',
			completed_at=?,
			updated_at=?
		WHERE id=? AND status NOT IN ('succeeded','failed','cancelled')`,
		now, now, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		if _, err := s.GetTask(id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) IsTaskCancelled(id string) (bool, error) {
	task, err := s.GetTask(id)
	if err != nil {
		return false, err
	}
	return task.Status == "cancelled", nil
}

func scanAccount(scanner interface {
	Scan(dest ...interface{}) error
}) (AccountRecord, error) {
	var a AccountRecord
	var enabled int
	err := scanner.Scan(&a.ID, &a.Name, &a.Type, &a.Status, &enabled, &a.Priority, &a.Weight,
		&a.CookieJSON, &a.CookieString, &a.LocalStorageJSON, &a.XsrfToken, &a.DeviceID,
		&a.UserAgent, &a.ProxyURL, &a.CapabilitiesJSON, &a.QuotaJSON, &a.LastError,
		&a.LastTestAt, &a.LastSuccessAt, &a.LastQuotaSyncAt, &a.CooldownUntil,
		&a.CreatedAt, &a.UpdatedAt)
	a.Enabled = enabled == 1
	return a, err
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultCapabilitiesForType(accountType string) string {
	if accountType == "guest" {
		return `{"chat":true,"image":false,"video":false}`
	}
	return `{"chat":true,"image":true,"video":true}`
}

func accountSupportsCapability(a AccountRecord, capability string) bool {
	if capability == "" {
		return true
	}
	if a.CapabilitiesJSON == "" {
		return a.Type != "guest" || capability == "chat"
	}
	var caps map[string]interface{}
	if err := json.Unmarshal([]byte(a.CapabilitiesJSON), &caps); err != nil {
		return false
	}
	value, ok := caps[capability]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == "true" || typed == "1" || typed == "yes"
	default:
		return false
	}
}
