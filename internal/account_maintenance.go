package internal

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultMaintenanceLease = 15 * time.Minute

type AccountMaintenance struct {
	AccountID      string `json:"account_id"`
	State          string `json:"state"`
	LeaseOwner     string `json:"lease_owner,omitempty"`
	LeaseExpiresAt string `json:"lease_expires_at,omitempty"`
	ProfilePath    string `json:"profile_path"`
	LastError      string `json:"last_error,omitempty"`
	UpdatedAt      string `json:"updated_at"`
}

func accountProfilePath(accountID string) string {
	return filepath.Join(Cfg.DataDir, "account-chrome-profiles", safeAccountPathSegment(accountID))
}

func safeAccountPathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func removeAccountProfile(accountID string) error {
	if strings.TrimSpace(accountID) == "" {
		return nil
	}
	return os.RemoveAll(accountProfilePath(accountID))
}

func (s *Store) BeginAccountMaintenance(accountID, owner string, ttl time.Duration) (*AccountMaintenance, error) {
	if _, err := s.GetAccount(accountID); err != nil {
		return nil, err
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return nil, errors.New("maintenance lease owner is required")
	}
	if ttl <= 0 {
		ttl = defaultMaintenanceLease
	}
	now := nowISO()
	expires := time.Now().UTC().Add(ttl).Format(time.RFC3339)
	result, err := s.db.Exec(`INSERT INTO qianwen_account_maintenance
		(account_id, state, lease_owner, lease_expires_at, profile_path, last_error, created_at, updated_at)
		VALUES (?, 'maintenance', ?, ?, ?, '', ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			state='maintenance', lease_owner=excluded.lease_owner,
			lease_expires_at=excluded.lease_expires_at, profile_path=excluded.profile_path,
			last_error='', updated_at=excluded.updated_at
		WHERE qianwen_account_maintenance.state NOT IN ('maintenance','validating')
			OR qianwen_account_maintenance.lease_expires_at < ?
			OR qianwen_account_maintenance.lease_owner = excluded.lease_owner`,
		accountID, owner, expires, accountProfilePath(accountID), now, now, now)
	if err != nil {
		return nil, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, fmt.Errorf("account %s already has an active maintenance lease", accountID)
	}
	return s.GetAccountMaintenance(accountID)
}

func (s *Store) HeartbeatAccountMaintenance(accountID, owner string, ttl time.Duration) (*AccountMaintenance, error) {
	if ttl <= 0 {
		ttl = defaultMaintenanceLease
	}
	result, err := s.db.Exec(`UPDATE qianwen_account_maintenance SET lease_expires_at=?, updated_at=?
		WHERE account_id=? AND lease_owner=? AND state IN ('maintenance','validating') AND lease_expires_at > ?`,
		time.Now().UTC().Add(ttl).Format(time.RFC3339), nowISO(), accountID, owner, nowISO())
	if err != nil {
		return nil, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, errors.New("maintenance lease is not owned by this session")
	}
	return s.GetAccountMaintenance(accountID)
}

func (s *Store) EndAccountMaintenance(accountID, owner, lastError string) error {
	result, err := s.db.Exec(`UPDATE qianwen_account_maintenance
		SET state='active', lease_owner='', lease_expires_at='', last_error=?, updated_at=?
		WHERE account_id=? AND (lease_owner=? OR lease_expires_at < ?)`, lastError, nowISO(), accountID, owner, nowISO())
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return errors.New("maintenance lease is not owned by this session")
	}
	return nil
}

func (s *Store) GetAccountMaintenance(accountID string) (*AccountMaintenance, error) {
	var record AccountMaintenance
	err := s.db.QueryRow(`SELECT account_id, state, COALESCE(lease_owner,''), COALESCE(lease_expires_at,''),
		COALESCE(profile_path,''), COALESCE(last_error,''), updated_at FROM qianwen_account_maintenance WHERE account_id=?`, accountID).
		Scan(&record.AccountID, &record.State, &record.LeaseOwner, &record.LeaseExpiresAt, &record.ProfilePath, &record.LastError, &record.UpdatedAt)
	if err == sql.ErrNoRows {
		return &AccountMaintenance{AccountID: accountID, State: "active", ProfilePath: accountProfilePath(accountID)}, nil
	}
	if err != nil {
		return nil, err
	}
	if record.State == "maintenance" || record.State == "validating" {
		expires, parseErr := time.Parse(time.RFC3339, record.LeaseExpiresAt)
		if parseErr != nil || !expires.After(time.Now().UTC()) {
			result, updateErr := s.db.Exec(`UPDATE qianwen_account_maintenance
				SET state='active', lease_owner='', lease_expires_at='', updated_at=?
				WHERE account_id=? AND lease_expires_at=? AND state IN ('maintenance','validating')`,
				nowISO(), accountID, record.LeaseExpiresAt)
			if updateErr != nil {
				return nil, updateErr
			}
			affected, _ := result.RowsAffected()
			if affected == 0 {
				return s.GetAccountMaintenance(accountID)
			}
			record.State = "active"
			record.LeaseOwner = ""
			record.LeaseExpiresAt = ""
		}
	}
	return &record, nil
}

func (s *Store) IsAccountInMaintenance(accountID string) (bool, error) {
	record, err := s.GetAccountMaintenance(accountID)
	if err != nil {
		return false, err
	}
	if record.State != "maintenance" && record.State != "validating" {
		return false, nil
	}
	return true, nil
}

func (s *Store) UpdateAccountSessionSnapshot(id, cookieJSON, cookieString, localStorageJSON, userAgent string) error {
	result, err := s.db.Exec(`UPDATE qianwen_accounts SET
		cookie_json=COALESCE(NULLIF(?,''), cookie_json), cookie_string=COALESCE(NULLIF(?,''), cookie_string),
		local_storage_json=COALESCE(NULLIF(?,''), local_storage_json), user_agent=COALESCE(NULLIF(?,''), user_agent),
		status='maintenance_pending_validation', last_error='Maintenance login captured; account validation is required.', updated_at=? WHERE id=?`,
		cookieJSON, cookieString, localStorageJSON, userAgent, nowISO(), id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
