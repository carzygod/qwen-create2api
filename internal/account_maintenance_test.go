package internal

import (
	"path/filepath"
	"testing"
	"time"
)

func newMaintenanceTestStore(t *testing.T) (*Store, AccountRecord) {
	t.Helper()
	oldCfg := Cfg
	oldStore := AppStore
	dir := t.TempDir()
	Cfg = &Config{
		DataDir:           dir,
		DatabasePath:      filepath.Join(dir, "qianwen-creator-maintenance.sqlite"),
		DefaultChatModel:  "qianwen-creator-session",
		DefaultImageModel: "qianwen-creator-wan27-image",
		DefaultVideoModel: QianwenVideoModelID,
	}
	if err := InitStore(); err != nil {
		t.Fatalf("InitStore() error = %v", err)
	}
	store := AppStore
	account := AccountRecord{
		Name:             "maintenance-test",
		Type:             "login_cookie",
		Status:           "valid",
		Enabled:          true,
		CookieString:     "tongyi_sso_ticket=test",
		CapabilitiesJSON: `{"chat":true,"image":true,"video":true}`,
	}
	if err := store.CreateAccount(&account); err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	t.Cleanup(func() {
		if store != nil && store.db != nil {
			_ = store.db.Close()
		}
		Cfg = oldCfg
		AppStore = oldStore
	})
	return store, account
}

func TestSafeAccountPathSegment(t *testing.T) {
	tests := map[string]string{
		"account-01":       "account-01",
		" account_02 ":     "account_02",
		"../../other-user": "______other-user",
		"账号 03":            "___03",
		"":                 "unknown",
	}
	for input, expected := range tests {
		if actual := safeAccountPathSegment(input); actual != expected {
			t.Fatalf("safeAccountPathSegment(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestMaintenanceLeaseBlocksSchedulingAndRequiresOwner(t *testing.T) {
	store, account := newMaintenanceTestStore(t)
	started, err := store.BeginAccountMaintenance(account.ID, "owner-a", time.Minute)
	if err != nil {
		t.Fatalf("BeginAccountMaintenance() error = %v", err)
	}
	if started.State != "maintenance" || started.LeaseOwner != "owner-a" {
		t.Fatalf("maintenance = %+v", started)
	}
	if _, err := store.SelectRunnableAccountForCapability("chat"); err == nil {
		t.Fatal("SelectRunnableAccountForCapability() succeeded during maintenance")
	}
	if _, err := store.HeartbeatAccountMaintenance(account.ID, "owner-b", time.Minute); err == nil {
		t.Fatal("heartbeat with foreign owner succeeded")
	}
	if err := store.EndAccountMaintenance(account.ID, "owner-b", ""); err == nil {
		t.Fatal("end with foreign owner succeeded")
	}
	if err := store.EndAccountMaintenance(account.ID, "owner-a", ""); err != nil {
		t.Fatalf("EndAccountMaintenance() error = %v", err)
	}
	selected, err := store.SelectRunnableAccountForCapability("chat")
	if err != nil {
		t.Fatalf("SelectRunnableAccountForCapability() after maintenance error = %v", err)
	}
	if selected.ID != account.ID {
		t.Fatalf("selected account = %q, want %q", selected.ID, account.ID)
	}
}

func TestCapturedSessionRequiresValidationBeforeScheduling(t *testing.T) {
	store, account := newMaintenanceTestStore(t)
	if _, err := store.BeginAccountMaintenance(account.ID, "owner-a", time.Minute); err != nil {
		t.Fatalf("BeginAccountMaintenance() error = %v", err)
	}
	if err := store.UpdateAccountSessionSnapshot(
		account.ID,
		`[{"name":"tongyi_sso_ticket","value":"new"}]`,
		"tongyi_sso_ticket=new",
		`{"access_token":"new"}`,
		"test-agent",
	); err != nil {
		t.Fatalf("UpdateAccountSessionSnapshot() error = %v", err)
	}
	if err := store.EndAccountMaintenance(account.ID, "owner-a", ""); err != nil {
		t.Fatalf("EndAccountMaintenance() error = %v", err)
	}
	updated, err := store.GetAccount(account.ID)
	if err != nil {
		t.Fatalf("GetAccount() error = %v", err)
	}
	if updated.Status != "maintenance_pending_validation" {
		t.Fatalf("status = %q, want maintenance_pending_validation", updated.Status)
	}
	if _, err := store.SelectRunnableAccountForCapability("chat"); err == nil {
		t.Fatal("captured account became runnable before validation")
	}
}

func TestExpiredMaintenanceLeaseCanBeTakenOver(t *testing.T) {
	store, account := newMaintenanceTestStore(t)
	if _, err := store.BeginAccountMaintenance(account.ID, "owner-a", time.Minute); err != nil {
		t.Fatalf("BeginAccountMaintenance() error = %v", err)
	}
	if _, err := store.db.Exec(
		"UPDATE qianwen_account_maintenance SET lease_expires_at=? WHERE account_id=?",
		time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		account.ID,
	); err != nil {
		t.Fatalf("expire lease error = %v", err)
	}
	started, err := store.BeginAccountMaintenance(account.ID, "owner-b", time.Minute)
	if err != nil {
		t.Fatalf("take over expired lease error = %v", err)
	}
	if started.LeaseOwner != "owner-b" {
		t.Fatalf("lease owner = %q, want owner-b", started.LeaseOwner)
	}
}
