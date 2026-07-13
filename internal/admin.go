package internal

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func HandleAdminPage(w http.ResponseWriter, r *http.Request) {
	if !requireAdminAuth(w, r) {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(adminHTML))
}

func HandleAdminAPI(w http.ResponseWriter, r *http.Request) {
	if !requireAdminAuth(w, r) {
		return
	}
	if AppStore == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store_not_ready", "SQLite store is not initialized.")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api")
	switch {
	case path == "/admin/summary" && r.Method == http.MethodGet:
		handleAdminSummary(w, r)
	case path == "/login-sessions" || strings.HasPrefix(path, "/login-sessions/"):
		handleLoginSessions(w, r, path)
	case path == "/accounts" && r.Method == http.MethodGet:
		handleListAccounts(w, r)
	case path == "/accounts" && r.Method == http.MethodPost:
		handleCreateAccount(w, r)
	case strings.HasPrefix(path, "/accounts/"):
		handleAccountAction(w, r, strings.TrimPrefix(path, "/accounts/"))
	case path == "/tasks" && r.Method == http.MethodGet:
		handleListTasks(w, r)
	case strings.HasPrefix(path, "/tasks/") && r.Method == http.MethodGet:
		handleGetTask(w, r, strings.TrimPrefix(path, "/tasks/"))
	case path == "/models" && r.Method == http.MethodGet:
		models, err := AppStore.ListModels()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "model_list_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"data": models})
	default:
		writeAPIError(w, http.StatusNotFound, "admin_route_not_found", "Admin API route not found.")
	}
}

func handleAdminSummary(w http.ResponseWriter, r *http.Request) {
	accounts, _ := AppStore.ListAccounts()
	tasks, _ := AppStore.ListTasks(200)
	accountStatus := map[string]int{}
	taskStatus := map[string]int{}
	for _, a := range accounts {
		accountStatus[a.Status]++
	}
	for _, t := range tasks {
		taskStatus[t.Status]++
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"service": map[string]interface{}{
			"name":            QianwenCreatorProviderCode,
			"host":            Cfg.Host,
			"port":            Cfg.Port,
			"data_dir":        Cfg.DataDir,
			"database_path":   Cfg.DatabasePath,
			"public_base_url": Cfg.PublicBaseURL,
			"guest_pool_size": Cfg.PoolSize,
		},
		"accounts": map[string]interface{}{
			"total":  len(accounts),
			"status": accountStatus,
		},
		"tasks": map[string]interface{}{
			"total":  len(tasks),
			"status": taskStatus,
		},
	})
}

func handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := AppStore.ListAccounts()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "account_list_failed", err.Error())
		return
	}
	if accounts == nil {
		accounts = []AccountRecord{}
	}
	for i := range accounts {
		accounts[i] = maskAccount(accounts[i])
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": accounts})
}

func handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string `json:"name"`
		CookieJSON       string `json:"cookie_json"`
		CookieString     string `json:"cookie_string"`
		LocalStorageJSON string `json:"local_storage_json"`
		UserAgent        string `json:"user_agent"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeAPIError(w, http.StatusBadRequest, "account_name_required", "Account name is required before generating a QR login session.")
		return
	}
	if strings.TrimSpace(req.CookieJSON) != "" || strings.TrimSpace(req.CookieString) != "" {
		account := &AccountRecord{
			Name:             name,
			Type:             "login_cookie",
			Status:           "unknown",
			Enabled:          true,
			CookieJSON:       strings.TrimSpace(req.CookieJSON),
			CookieString:     strings.TrimSpace(req.CookieString),
			LocalStorageJSON: strings.TrimSpace(req.LocalStorageJSON),
			UserAgent:        defaultString(strings.TrimSpace(req.UserAgent), generateRandomUserAgent()),
			CapabilitiesJSON: `{"chat":true,"image":true,"video":true}`,
			LastError:        "Cookie material imported. Run account test before routing traffic to this account.",
		}
		if err := AppStore.CreateAccount(account); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "account_create_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"data": maskAccount(*account),
			"next": "test_account",
		})
		return
	}
	session, err := QianwenLoginSessions.Start(name)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "login_session_start_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"data": session,
		"next": "scan_qr_then_confirm_capture",
	})
}

func handleAccountAction(w http.ResponseWriter, r *http.Request, suffix string) {
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeAPIError(w, http.StatusNotFound, "account_route_not_found", "Account route not found.")
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			account, err := AppStore.GetAccount(id)
			if err != nil {
				writeAccountLookupError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"data": maskAccount(*account)})
		case http.MethodDelete:
			QianwenLoginSessions.DeleteByAccountID(id)
			if err := AppStore.DeleteAccount(id); err != nil {
				writeAPIError(w, http.StatusInternalServerError, "account_delete_failed", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		default:
			writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
		}
		return
	}
	if len(parts) >= 2 && parts[1] == "maintenance" {
		handleAccountMaintenanceAction(w, r, id, parts[2:])
		return
	}
	if len(parts) == 2 && parts[1] == "test" && r.Method == http.MethodPost {
		var body struct {
			Capability string `json:"capability"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		result, err := TestAccount(id, body.Capability)
		if err != nil {
			writeAccountLookupError(w, err)
			return
		}
		status := http.StatusOK
		if !result.OK {
			status = http.StatusFailedDependency
		}
		writeJSON(w, status, result)
		return
	}
	if len(parts) == 3 && parts[1] == "quota" && parts[2] == "sync" && r.Method == http.MethodPost {
		msg := "Quota sync requires create.qianwen.com logged-in quota endpoint capture. No quota was changed."
		_ = AppStore.UpdateAccountStatus(id, "unknown", msg, false)
		writeJSON(w, http.StatusFailedDependency, map[string]interface{}{
			"ok":      false,
			"code":    "quota_protocol_required",
			"message": msg,
		})
		return
	}
	writeAPIError(w, http.StatusNotFound, "account_route_not_found", "Account route not found.")
}

func handleAccountMaintenanceAction(w http.ResponseWriter, r *http.Request, accountID string, parts []string) {
	action := "status"
	if len(parts) > 0 && parts[0] != "" {
		action = parts[0]
	}
	switch action {
	case "status":
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for maintenance status.")
			return
		}
		record, err := AppStore.GetAccountMaintenance(accountID)
		if err != nil {
			writeAccountLookupError(w, err)
			return
		}
		if account, accountErr := AppStore.GetAccount(accountID); accountErr == nil && record.State == "active" && account.Status == "maintenance_pending_validation" {
			record.State = "validating"
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"data": record})
	case "start":
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST to start maintenance.")
			return
		}
		session, err := QianwenLoginSessions.StartMaintenance(accountID)
		if err != nil {
			writeAPIError(w, http.StatusConflict, "maintenance_start_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{"data": session})
	case "heartbeat":
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for maintenance heartbeat.")
			return
		}
		var body struct{ LeaseOwner string `json:"lease_owner"` }
		if err := decodeJSON(r, &body); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
		record, err := AppStore.HeartbeatAccountMaintenance(accountID, body.LeaseOwner, defaultMaintenanceLease)
		if err != nil {
			writeAPIError(w, http.StatusConflict, "maintenance_heartbeat_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"data": record})
	case "stop":
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST to stop maintenance.")
			return
		}
		record, err := AppStore.GetAccountMaintenance(accountID)
		if err != nil {
			writeAccountLookupError(w, err)
			return
		}
		if record.LeaseOwner != "" && QianwenLoginSessions.Delete(record.LeaseOwner) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "state": "active"})
			return
		}
		if record.LeaseOwner != "" {
			writeAPIError(w, http.StatusConflict, "maintenance_owned_elsewhere", "The active maintenance lease is owned by another process.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "state": "active"})
	case "validate":
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST to validate maintenance.")
			return
		}
		active, err := AppStore.IsAccountInMaintenance(accountID)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "maintenance_status_failed", err.Error())
			return
		}
		if active {
			writeAPIError(w, http.StatusConflict, "maintenance_active", "Stop the maintenance browser before validation.")
			return
		}
		result, err := TestAccount(accountID, "")
		if err != nil {
			writeAccountLookupError(w, err)
			return
		}
		if !result.OK {
			_ = AppStore.UpdateAccountStatus(accountID, "maintenance_pending_validation", result.Message, false)
			writeJSON(w, http.StatusFailedDependency, result)
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		writeAPIError(w, http.StatusNotFound, "maintenance_action_not_found", "Unknown maintenance action.")
	}
}

func handleListTasks(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	tasks, err := AppStore.ListTasks(limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "task_list_failed", err.Error())
		return
	}
	if tasks == nil {
		tasks = []TaskRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": tasks})
}

func handleGetTask(w http.ResponseWriter, r *http.Request, id string) {
	task, err := AppStore.GetTask(strings.Trim(id, "/"))
	if err != nil {
		if err == sql.ErrNoRows {
			writeAPIError(w, http.StatusNotFound, "task_not_found", "Task not found.")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "task_get_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func writeAccountLookupError(w http.ResponseWriter, err error) {
	if err == sql.ErrNoRows {
		writeAPIError(w, http.StatusNotFound, "account_not_found", "Account not found.")
		return
	}
	writeAPIError(w, http.StatusInternalServerError, "account_lookup_failed", err.Error())
}

func maskAccount(a AccountRecord) AccountRecord {
	a.CookieJSON = maskSecret(a.CookieJSON)
	a.CookieString = maskSecret(a.CookieString)
	a.LocalStorageJSON = maskSecret(a.LocalStorageJSON)
	a.XsrfToken = maskSecret(a.XsrfToken)
	return a
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 12 {
		return "***"
	}
	return value[:6] + "..." + value[len(value)-4:]
}

const legacyAdminHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>QIANWEN-CREATOR-01 Admin</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #0b1013;
      --surface: #11181d;
      --surface-2: #172026;
      --surface-3: #1c2930;
      --text: #ecf4f1;
      --muted: #8fa29b;
      --line: rgba(156, 190, 178, .18);
      --accent: #34e0a1;
      --accent-2: #8fc7ff;
      --warn: #ffd166;
      --danger: #ff6b7c;
      --shadow: 0 22px 60px rgba(0, 0, 0, .32);
      --radius: 22px;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      background:
        radial-gradient(circle at 12% 0%, rgba(52, 224, 161, .14), transparent 34rem),
        linear-gradient(135deg, #091013 0%, #0d1519 48%, #10161d 100%);
      color: var(--text);
      font: 14px/1.5 Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      letter-spacing: 0;
    }
    header {
      position: sticky;
      top: 0;
      z-index: 5;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      padding: 18px 28px;
      background: rgba(9, 16, 19, .84);
      border-bottom: 1px solid var(--line);
      backdrop-filter: blur(18px);
    }
    h1, h2, h3, p { margin: 0; }
    h1 { font-size: 19px; font-weight: 760; }
    h2 { font-size: 15px; font-weight: 720; }
    h3 { font-size: 13px; color: var(--muted); font-weight: 650; }
    main { width: min(1420px, calc(100vw - 36px)); margin: 0 auto; padding: 24px 0 44px; }
    .header-actions { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
    .pill {
      display: inline-flex;
      align-items: center;
      min-height: 32px;
      border: 1px solid var(--line);
      border-radius: 999px;
      color: var(--muted);
      background: rgba(255, 255, 255, .03);
      padding: 0 12px;
      font-size: 12px;
      white-space: nowrap;
    }
    .metrics {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 14px;
    }
    .metric-card, section {
      background: linear-gradient(180deg, rgba(255, 255, 255, .035), rgba(255, 255, 255, .015)), var(--surface);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      box-shadow: var(--shadow);
    }
    .metric-card {
      min-height: 106px;
      padding: 17px 18px;
      overflow: hidden;
    }
    .metric-label { color: var(--muted); font-size: 12px; }
    .metric-value { margin-top: 10px; font-size: 28px; font-weight: 780; }
    .metric-note { margin-top: 4px; color: var(--muted); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    section { margin-top: 18px; overflow: hidden; }
    .section-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 14px;
      padding: 16px 18px;
      border-bottom: 1px solid var(--line);
    }
    .section-actions { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
    button, input, select {
      min-height: 38px;
      border: 1px solid var(--line);
      border-radius: 14px;
      background: var(--surface-2);
      color: var(--text);
      padding: 8px 12px;
      font: inherit;
      letter-spacing: 0;
    }
    button {
      cursor: pointer;
      transition: transform .18s ease, border-color .18s ease, background .18s ease, box-shadow .18s ease;
      user-select: none;
    }
    button:hover { transform: translateY(-1px); border-color: rgba(52, 224, 161, .58); box-shadow: 0 10px 28px rgba(0, 0, 0, .22); }
    button.primary {
      color: #03110c;
      background: linear-gradient(135deg, var(--accent), #78f2c8);
      border-color: rgba(52, 224, 161, .9);
      font-weight: 760;
    }
    button.danger { color: var(--danger); border-color: rgba(255, 107, 124, .38); }
    button.subtle { color: var(--muted); }
    table { width: 100%; border-collapse: collapse; table-layout: fixed; }
    th, td { text-align: left; padding: 12px 14px; border-bottom: 1px solid var(--line); vertical-align: top; }
    th { color: var(--muted); font-size: 12px; font-weight: 680; background: rgba(255, 255, 255, .025); }
    td { word-break: break-word; }
    tr:last-child td { border-bottom: 0; }
    code { color: #b9ffe1; font-size: 12px; }
    .status { font-weight: 720; }
    .ok { color: var(--accent); }
    .bad { color: var(--danger); }
    .warn { color: var(--warn); }
    .muted { color: var(--muted); }
    .inline-actions { display: flex; gap: 8px; flex-wrap: wrap; }
    .empty { padding: 26px 18px; color: var(--muted); text-align: center; }
    dialog {
      width: min(800px, calc(100vw - 30px));
      border: 1px solid var(--line);
      border-radius: 24px;
      background: linear-gradient(180deg, rgba(255,255,255,.04), rgba(255,255,255,.015)), var(--surface);
      color: var(--text);
      box-shadow: 0 34px 110px rgba(0, 0, 0, .62);
      padding: 0;
      overflow: hidden;
    }
    dialog::backdrop { background: rgba(0, 0, 0, .62); backdrop-filter: blur(10px); }
    .dialog-body { padding: 20px; }
    .dialog-head { display: flex; justify-content: space-between; gap: 16px; padding: 18px 20px; border-bottom: 1px solid var(--line); }
    .dialog-actions { display: flex; justify-content: flex-end; gap: 10px; flex-wrap: wrap; margin-top: 18px; }
    .field { display: grid; gap: 7px; margin-top: 16px; }
    .field label { color: var(--muted); font-size: 12px; }
    .field input { width: 100%; }
    .guide {
      display: grid;
      gap: 9px;
      margin-top: 16px;
      padding: 14px;
      border: 1px solid var(--line);
      border-radius: 18px;
      background: rgba(52, 224, 161, .055);
      color: var(--muted);
      font-size: 13px;
    }
    .qr-frame {
      margin-top: 16px;
      min-height: 410px;
      border: 1px solid var(--line);
      border-radius: 20px;
      background: #060a0d;
      display: grid;
      place-items: center;
      overflow: hidden;
    }
    .qr-frame img { width: 100%; max-height: 620px; object-fit: contain; display: block; }
    .toast {
      position: fixed;
      left: 50%;
      bottom: 24px;
      transform: translateX(-50%) translateY(20px);
      opacity: 0;
      pointer-events: none;
      padding: 11px 14px;
      border-radius: 14px;
      background: #0d1713;
      border: 1px solid rgba(52, 224, 161, .38);
      box-shadow: var(--shadow);
      transition: opacity .2s ease, transform .2s ease;
      z-index: 20;
    }
    .toast.show { opacity: 1; transform: translateX(-50%) translateY(0); }
    @media (max-width: 1000px) {
      .metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); }
      header { align-items: flex-start; flex-direction: column; }
      table { table-layout: auto; }
    }
    @media (max-width: 680px) {
      main { width: min(100vw - 20px, 1420px); }
      .metrics { grid-template-columns: 1fr; }
      .section-head, .dialog-head { align-items: flex-start; flex-direction: column; }
      th, td { padding: 10px; }
    }
  </style>
</head>
<body>
  <header>
    <div>
      <h1>QIANWEN-CREATOR-01 Admin</h1>
      <p class="muted">QR account pool, SQLite storage, no Redis.</p>
    </div>
    <div class="header-actions">
      <span class="pill" id="servicePill">loading</span>
      <button class="primary" onclick="openAccountDialog()">Add account</button>
      <button onclick="loadAll()">Refresh</button>
    </div>
  </header>
  <main>
    <div class="metrics">
      <div class="metric-card"><div class="metric-label">Accounts</div><div class="metric-value" id="accountTotal">-</div><div class="metric-note" id="accountBreakdown">waiting</div></div>
      <div class="metric-card"><div class="metric-label">Tasks</div><div class="metric-value" id="taskTotal">-</div><div class="metric-note" id="taskBreakdown">waiting</div></div>
      <div class="metric-card"><div class="metric-label">Guest pool</div><div class="metric-value" id="guestPool">-</div><div class="metric-note">disabled when set to 0</div></div>
      <div class="metric-card"><div class="metric-label">Port</div><div class="metric-value" id="port">-</div><div class="metric-note" id="publicUrl">public URL</div></div>
    </div>

    <section>
      <div class="section-head">
        <div>
          <h2>Accounts</h2>
          <p class="muted">Saved accounts come only from confirmed QR login sessions.</p>
        </div>
        <div class="section-actions">
          <button class="primary" onclick="openAccountDialog()">Add account</button>
          <button onclick="loadAccounts()">Refresh accounts</button>
        </div>
      </div>
      <div style="overflow:auto;">
        <table>
          <thead><tr><th style="width:22%;">Name</th><th style="width:12%;">Type</th><th style="width:11%;">Status</th><th style="width:18%;">Capabilities</th><th>Last message</th><th style="width:18%;">Actions</th></tr></thead>
          <tbody id="accounts"><tr><td colspan="6" class="empty">Loading accounts</td></tr></tbody>
        </table>
      </div>
    </section>

    <section>
      <div class="section-head">
        <div>
          <h2>QR login sessions</h2>
          <p class="muted">Refresh creates a new QR browser session. Delete closes Chromium and removes its profile.</p>
        </div>
        <div class="section-actions">
          <button onclick="loadLoginSessions()">Refresh sessions</button>
        </div>
      </div>
      <div style="overflow:auto;">
        <table>
          <thead><tr><th style="width:23%;">Session</th><th style="width:13%;">Status</th><th style="width:10%;">Cookies</th><th>Message</th><th style="width:24%;">Actions</th></tr></thead>
          <tbody id="loginSessions"><tr><td colspan="5" class="empty">Loading sessions</td></tr></tbody>
        </table>
      </div>
    </section>

    <section>
      <div class="section-head">
        <div>
          <h2>Tasks</h2>
          <p class="muted">Recent chat, image, and video task records.</p>
        </div>
        <button onclick="loadTasks()">Refresh tasks</button>
      </div>
      <div style="overflow:auto;">
        <table>
          <thead><tr><th style="width:23%;">ID</th><th style="width:10%;">Type</th><th style="width:16%;">Model</th><th style="width:12%;">Status</th><th>Error</th><th style="width:16%;">Created</th></tr></thead>
          <tbody id="tasks"><tr><td colspan="6" class="empty">Loading tasks</td></tr></tbody>
        </table>
      </div>
    </section>
  </main>

  <dialog id="accountDialog">
    <form method="dialog" onsubmit="event.preventDefault(); createAccount();">
      <div class="dialog-head">
        <div>
          <h2>Add Qianwen account</h2>
          <p class="muted">Start with a name, then generate a QR login browser.</p>
        </div>
        <button type="button" class="subtle" onclick="accountDialog.close()">Close</button>
      </div>
      <div class="dialog-body">
        <div class="guide">
          <div>1. Enter a readable account name.</div>
          <div>2. Generate QR. The server opens Chromium in the background.</div>
          <div>3. Scan the QR in the screenshot with your Qianwen/Taobao/Alipay login flow.</div>
          <div>4. After the page is logged in, click Confirm scan to save the account into SQLite.</div>
        </div>
        <div class="field">
          <label for="accountName">Account name</label>
          <input id="accountName" required autocomplete="off" placeholder="Qianwen main account" />
        </div>
        <div class="dialog-actions">
          <button type="button" onclick="accountDialog.close()">Cancel</button>
          <button class="primary" type="submit">Generate QR</button>
        </div>
      </div>
    </form>
  </dialog>

  <dialog id="loginDialog">
    <div class="dialog-head">
      <div>
        <h2>Scan QR login</h2>
        <p class="muted" id="loginSessionName">Waiting for session</p>
      </div>
      <button type="button" class="subtle" onclick="closeLoginDialog()">Close</button>
    </div>
    <div class="dialog-body">
      <div class="guide">
        <div>Scan the QR shown in the screenshot. QR codes expire quickly; use Refresh QR when it is stale.</div>
        <div>Confirm scan only after the screenshot shows a logged-in Qianwen page.</div>
      </div>
      <div class="qr-frame">
        <img id="loginShot" alt="Qianwen login screenshot" />
      </div>
      <p class="muted" id="loginStatusText" style="margin-top:12px;">Loading QR session</p>
      <div class="dialog-actions">
        <button type="button" onclick="clickLoginEntry()">Click login entry</button>
        <button type="button" onclick="refreshCurrentLoginSession()">Refresh QR</button>
        <button type="button" onclick="captureCurrentLoginSession()">Confirm scan</button>
        <button type="button" class="danger" onclick="deleteCurrentLoginSession()">Delete session</button>
      </div>
    </div>
  </dialog>

  <div class="toast" id="toast"></div>

  <script>
    const initialKey = new URLSearchParams(location.search).get('key') || localStorage.getItem('qianwenAdminKey') || '';
    if (initialKey) localStorage.setItem('qianwenAdminKey', initialKey);
    const adminKey = initialKey;
    let currentLoginSessionId = '';
    let loginPollTimer = 0;

    function $(id) { return document.getElementById(id); }
    function headers() { return { 'Content-Type': 'application/json', 'X-Admin-Key': adminKey }; }
    async function api(path, options) {
      const opts = options || {};
      const res = await fetch('/api' + path, Object.assign({}, opts, { headers: Object.assign(headers(), opts.headers || {}) }));
      const text = await res.text();
      let data = {};
      if (text) {
        try { data = JSON.parse(text); } catch (err) { data = { message: text }; }
      }
      if (!res.ok) throw new Error(data.message || (data.error && data.error.message) || res.statusText);
      return data;
    }
    function screenshotUrl(sessionId) {
      return '/api/login-sessions/' + encodeURIComponent(sessionId) + '/screenshot?key=' + encodeURIComponent(adminKey) + '&t=' + Date.now();
    }
    function openAccountDialog() {
      $('accountName').value = '';
      accountDialog.showModal();
      setTimeout(function(){ $('accountName').focus(); }, 60);
    }
    async function createAccount() {
      const name = $('accountName').value.trim();
      if (!name) {
        toastMessage('Enter an account name first.');
        return;
      }
      const data = await api('/accounts', { method: 'POST', body: JSON.stringify({ name: name }) });
      accountDialog.close();
      currentLoginSessionId = data.data.id;
      showLoginDialog(data.data);
      await loadAll();
    }
    async function loadAll() {
      await Promise.all([loadSummary(), loadAccounts(), loadLoginSessions(), loadTasks()]);
    }
    async function loadSummary() {
      const summary = await api('/admin/summary');
      $('accountTotal').textContent = summary.accounts.total;
      $('taskTotal').textContent = summary.tasks.total;
      $('guestPool').textContent = summary.service.guest_pool_size;
      $('port').textContent = summary.service.port;
      $('servicePill').textContent = summary.service.name + ' on 0.0.0.0:' + summary.service.port;
      $('publicUrl').textContent = summary.service.public_base_url || location.origin;
      $('accountBreakdown').textContent = breakdown(summary.accounts.status);
      $('taskBreakdown').textContent = breakdown(summary.tasks.status);
    }
    async function loadAccounts() {
      const result = await api('/accounts');
      const rows = result.data || [];
      if (!rows.length) {
        $('accounts').innerHTML = '<tr><td colspan="6" class="empty">No accounts yet. Click Add account to start a QR login.</td></tr>';
        return;
      }
      $('accounts').innerHTML = rows.map(function(a) {
        return '<tr><td><strong>' + esc(a.name) + '</strong><br><code>' + esc(a.id) + '</code></td>' +
          '<td>' + esc(a.type) + '</td>' +
          '<td>' + status(a.status) + '</td>' +
          '<td><code>' + esc(a.capabilities_json || '') + '</code></td>' +
          '<td>' + esc(a.last_error || '') + '</td>' +
          '<td><div class="inline-actions"><button onclick="startMaintenance(\'' + escAttr(a.id) + '\')">Maintain</button><button onclick="testAccount(\'' + escAttr(a.id) + '\')">Test</button><button class="danger" onclick="deleteAccount(\'' + escAttr(a.id) + '\')">Delete</button></div></td></tr>';
      }).join('');
    }
    async function loadLoginSessions() {
      const result = await api('/login-sessions');
      const rows = result.data || [];
      if (!rows.length) {
        $('loginSessions').innerHTML = '<tr><td colspan="5" class="empty">No QR login sessions.</td></tr>';
        return;
      }
      $('loginSessions').innerHTML = rows.map(function(s) {
        return '<tr><td><strong>' + esc(s.name) + '</strong><br><code>' + esc(s.id) + '</code><br><span class="muted">' + esc(s.updated_at || '') + '</span></td>' +
          '<td>' + status(s.status) + '</td>' +
          '<td>' + esc(s.cookie_count || 0) + '</td>' +
          '<td>' + esc(s.message || '') + '</td>' +
          '<td><div class="inline-actions"><button onclick="showLoginSession(\'' + escAttr(s.id) + '\')">Open</button>' + (s.novnc_url ? '<button onclick="openNoVNC(\'' + escAttr(s.novnc_url) + '\')">noVNC</button>' : '') + '<button onclick="refreshLoginSession(\'' + escAttr(s.id) + '\')">Refresh QR</button><button onclick="captureLoginSession(\'' + escAttr(s.id) + '\')">Confirm</button><button class="danger" onclick="deleteLoginSession(\'' + escAttr(s.id) + '\')">Delete</button></div></td></tr>';
      }).join('');
    }
    async function loadTasks() {
      const result = await api('/tasks?limit=50');
      const rows = result.data || [];
      if (!rows.length) {
        $('tasks').innerHTML = '<tr><td colspan="6" class="empty">No tasks recorded yet.</td></tr>';
        return;
      }
      $('tasks').innerHTML = rows.map(function(t) {
        return '<tr><td><code>' + esc(t.id) + '</code></td><td>' + esc(t.type) + '</td><td>' + esc(t.model || '') + '</td><td>' + status(t.status) + '</td><td>' + esc(t.error_message || '') + '</td><td>' + esc(t.created_at || '') + '</td></tr>';
      }).join('');
    }
    async function showLoginSession(id) {
      const data = await api('/login-sessions/' + encodeURIComponent(id));
      showLoginDialog(data.data);
    }
    function openNoVNC(url) {
      if (url) window.open(url, '_blank', 'noopener');
    }
    async function startMaintenance(id) {
      try {
        const result = await api('/accounts/' + encodeURIComponent(id) + '/maintenance/start', { method: 'POST' });
        showLoginDialog(result.data);
        openNoVNC(result.data.novnc_url);
      } catch (err) {
        toastMessage(err.message);
      }
      await loadAll();
    }
    function showLoginDialog(session) {
      currentLoginSessionId = session.id;
      $('loginSessionName').textContent = session.name + ' / ' + session.id;
      updateLoginStatus(session);
      $('loginShot').src = screenshotUrl(session.id);
      loginDialog.showModal();
      startLoginPolling();
    }
    function closeLoginDialog() {
      loginDialog.close();
      stopLoginPolling();
    }
    function startLoginPolling() {
      stopLoginPolling();
      loginPollTimer = setInterval(async function() {
        if (!currentLoginSessionId || !loginDialog.open) return;
        try {
          const latest = await api('/login-sessions/' + encodeURIComponent(currentLoginSessionId));
          updateLoginStatus(latest.data);
          $('loginShot').src = screenshotUrl(currentLoginSessionId);
          await loadLoginSessions();
        } catch (err) {
          $('loginStatusText').textContent = err.message;
        }
      }, 5000);
    }
    function stopLoginPolling() {
      if (loginPollTimer) clearInterval(loginPollTimer);
      loginPollTimer = 0;
    }
    function updateLoginStatus(session) {
      $('loginStatusText').textContent = session.status + ' / cookies=' + (session.cookie_count || 0) + ' / ' + (session.message || '');
    }
    async function refreshLoginSession(id) {
      const data = await api('/login-sessions/' + encodeURIComponent(id) + '/refresh', { method: 'POST' });
      toastMessage('QR session refreshed. A new browser is starting.');
      if (currentLoginSessionId === id && loginDialog.open) {
        updateLoginStatus(data.data);
        $('loginShot').removeAttribute('src');
        setTimeout(function(){ $('loginShot').src = screenshotUrl(id); }, 3500);
      }
      await loadLoginSessions();
    }
    async function refreshCurrentLoginSession() {
      if (!currentLoginSessionId) return;
      await refreshLoginSession(currentLoginSessionId);
    }
    async function clickLoginEntry() {
      if (!currentLoginSessionId) return;
      await api('/login-sessions/' + encodeURIComponent(currentLoginSessionId) + '/click-login', { method: 'POST' });
      $('loginShot').src = screenshotUrl(currentLoginSessionId);
      await loadLoginSessions();
    }
    async function captureLoginSession(id) {
      try {
        const result = await api('/login-sessions/' + encodeURIComponent(id) + '/capture', { method: 'POST' });
        toastMessage('Account saved: ' + result.data.name);
        if (currentLoginSessionId === id && loginDialog.open) closeLoginDialog();
      } catch (err) {
        toastMessage(err.message);
      }
      await loadAll();
    }
    async function captureCurrentLoginSession() {
      if (!currentLoginSessionId) return;
      await captureLoginSession(currentLoginSessionId);
    }
    async function deleteLoginSession(id) {
      if (!confirm('Delete this QR session and close its Chromium process?')) return;
      await api('/login-sessions/' + encodeURIComponent(id), { method: 'DELETE' });
      if (currentLoginSessionId === id && loginDialog.open) closeLoginDialog();
      toastMessage('QR session deleted.');
      await loadLoginSessions();
    }
    async function deleteCurrentLoginSession() {
      if (!currentLoginSessionId) return;
      await deleteLoginSession(currentLoginSessionId);
    }
    async function testAccount(id) {
      try {
        const result = await api('/accounts/' + encodeURIComponent(id) + '/test', { method: 'POST', body: JSON.stringify({ capability: 'chat' }) });
        toastMessage(result.message || 'Account test passed.');
      } catch (err) {
        toastMessage(err.message);
      }
      await loadAll();
    }
    async function deleteAccount(id) {
      if (!confirm('Delete this saved account?')) return;
      await api('/accounts/' + encodeURIComponent(id), { method: 'DELETE' });
      toastMessage('Account deleted.');
      await loadAll();
    }
    function breakdown(obj) {
      const keys = Object.keys(obj || {});
      if (!keys.length) return 'none';
      return keys.map(function(k){ return k + ':' + obj[k]; }).join(' / ');
    }
    function status(value) {
      const v = value || 'unknown';
      const cls = (v === 'valid' || v === 'succeeded' || v === 'captured') ? 'ok' : ((v === 'invalid' || v === 'failed' || v === 'expired') ? 'bad' : 'warn');
      return '<span class="status ' + cls + '">' + esc(v) + '</span>';
    }
    function toastMessage(message) {
      $('toast').textContent = message;
      $('toast').classList.add('show');
      setTimeout(function(){ $('toast').classList.remove('show'); }, 2600);
    }
    function esc(value) {
      return String(value == null ? '' : value).replace(/[&<>"']/g, function(s) {
        return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#039;'}[s];
      });
    }
    function escAttr(value) { return esc(value).replace(/\\/g, '\\\\'); }
    loadAll().catch(function(err){ toastMessage(err.message); });
  </script>
</body>
</html>`
