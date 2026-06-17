package internal

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

type AccountData struct {
	ID          int    `json:"id"`
	BxUmidtoken string `json:"bx_umidtoken"`
}

type PoolData struct {
	LastRefresh time.Time     `json:"last_refresh"`
	Accounts    []AccountData `json:"accounts"`
}

const (
	umidFile = "umid.json"
)

func SaveAccounts(accounts []*Account, lastRefresh time.Time) error {
	if err := os.MkdirAll(Cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %v", err)
	}

	var accountsData []AccountData
	for _, acc := range accounts {
		acc.mu.Lock()
		accountsData = append(accountsData, AccountData{
			ID:          acc.ID,
			BxUmidtoken: acc.BxUmidtoken,
		})
		acc.mu.Unlock()
	}

	poolData := PoolData{
		LastRefresh: lastRefresh,
		Accounts:    accountsData,
	}

	data, err := json.MarshalIndent(poolData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal accounts: %v", err)
	}

	filePath := filepath.Join(Cfg.DataDir, umidFile)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write accounts file: %v", err)
	}

	LogInfo("Accounts saved to %s (last_refresh: %v)", filePath, lastRefresh)
	return nil
}

func LoadAccounts() (*PoolData, error) {
	filePath := filepath.Join(Cfg.DataDir, umidFile)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read accounts file: %v", err)
	}

	var poolData PoolData
	if err := json.Unmarshal(data, &poolData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal accounts: %v", err)
	}

	LogInfo("Loaded %d accounts from %s (last_refresh: %v)", len(poolData.Accounts), filePath, poolData.LastRefresh)
	return &poolData, nil
}

func RestoreAccount(data AccountData, lastRefresh time.Time) (*Account, error) {
	LogInfo("Account %d: restoring from saved data", data.ID)

	account := &Account{
		ID:          data.ID,
		BxUmidtoken: data.BxUmidtoken,
		LastRefresh: lastRefresh,
	}

	if err := refreshAccountTokens(account, false); err != nil {
		return nil, fmt.Errorf("failed to refresh account tokens: %v", err)
	}

	LogInfo("Account %d: restored successfully with %d bacsft tokens", account.ID, len(account.EoCltBacsft))
	return account, nil
}

func refreshAccountTokens(account *Account, refreshUmid bool) error {
	maxRetries := 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			LogInfo("Account %d: retry attempt %d/%d", account.ID, attempt, maxRetries)
			time.Sleep(time.Duration(1000+rand.Intn(1000)) * time.Millisecond)
		}

		err := refreshAccountTokensOnce(account, refreshUmid)
		if err == nil {
			if attempt > 1 {
				LogInfo("Account %d: succeeded on attempt %d", account.ID, attempt)
			}
			return nil
		}

		lastErr = err
		LogWarn("Account %d: attempt %d/%d failed: %v", account.ID, attempt, maxRetries, err)
	}

	return fmt.Errorf("failed after %d attempts: %v", maxRetries, lastErr)
}

func refreshAccountTokensOnce(account *Account, refreshUmid bool) error {
	if refreshUmid {
		umidToken, err := GenerateUMIDTokenWithRetry(3)
		if err != nil {
			return fmt.Errorf("failed to refresh UMID token: %v", err)
		}
		account.BxUmidtoken = umidToken
		LogDebug("Account %d: UMID token refreshed", account.ID)
	}

	client, err := NewQianwenClient()
	if err != nil {
		return fmt.Errorf("failed to create qianwen client: %v", err)
	}
	account.Client = client
	account.DeviceID = client.deviceID

	if err := account.Client.GetXSRFToken(); err != nil {
		return fmt.Errorf("failed to get XSRF token: %v", err)
	}
	account.XsrfToken = account.Client.xsrfToken
	LogDebug("Account %d: XSRF token obtained", account.ID)

	guestTicket, err := account.Client.GetGuestTicket()
	if err != nil {
		return fmt.Errorf("failed to get guest ticket: %v", err)
	}
	account.TongyiGuestTicket = guestTicket
	LogDebug("Account %d: Guest ticket obtained", account.ID)

	registerResp, err := account.Client.RegisterAndGetTokens(account.BxUmidtoken)
	if err != nil {
		return fmt.Errorf("failed to register: %v", err)
	}

	account.EoCltDvidn = registerResp.Data.EoCltDvidn
	account.EoCltSnver = registerResp.Data.EoCltSnver
	account.EoCltBacsft = registerResp.Data.EoCltBacsft

	if len(registerResp.Data.UnifyRelate) > 0 {
		for _, relate := range registerResp.Data.UnifyRelate {
			if relate.BusinessScene == "qwen_business" {
				account.EoCltActkn = relate.EoCltActkn
				if len(relate.EoCltBacsft) > 0 {
					account.EoCltBacsft = relate.EoCltBacsft
				}
				break
			}
		}
	}

	LogDebug("Account %d: Registration completed, bacsft count: %d", account.ID, len(account.EoCltBacsft))
	return nil
}
