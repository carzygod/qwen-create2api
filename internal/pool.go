package internal

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type Account struct {
	ID                int
	TongyiGuestTicket string
	BxUmidtoken       string
	EoCltDvidn        string
	EoCltActkn        string
	EoCltSnver        string
	EoCltBacsft       []string
	XsrfToken         string
	DeviceID          string
	Client            *QianwenClient
	InUse             bool
	LastRefresh       time.Time
	mu                sync.Mutex
}

type AccountPool struct {
	accounts    []*Account
	availableCh chan *Account
	mu          sync.Mutex
}

var GlobalPool *AccountPool
var GuestPoolInitError string

func InitPool(size int) error {
	GuestPoolInitError = ""
	LogInfo("Initializing account pool with size: %d", size)

	GlobalPool = &AccountPool{
		accounts:    make([]*Account, 0, size),
		availableCh: make(chan *Account, size),
	}

	if size == 0 {
		LogWarn("Guest account pool disabled because POOL_SIZE=0")
		return nil
	}

	poolData, err := LoadAccounts()
	if err != nil {
		LogWarn("Failed to load accounts from file: %v", err)
	}

	needRefresh := false
	if poolData == nil || len(poolData.Accounts) != size {
		if poolData != nil {
			LogInfo("Pool size changed (%d -> %d), creating new accounts", len(poolData.Accounts), size)
		}
		needRefresh = true
	} else {
		if time.Since(poolData.LastRefresh) > Cfg.RefreshPeriod {
			LogInfo("Accounts expired (last refresh: %v), creating new accounts", poolData.LastRefresh)
			needRefresh = true
		}
	}

	lastRefresh := time.Now()

	if needRefresh || poolData == nil {
		umidTokens, err := GenerateBatchUMIDTokens(size)
		if err != nil {
			return fmt.Errorf("failed to generate UMID tokens: %v", err)
		}

		LogInfo("Creating %d accounts with generated UMID tokens...", size)

		for i := 0; i < size; i++ {
			LogInfo("Creating account %d/%d...", i+1, size)
			account, err := createAccountWithUmid(i+1, umidTokens[i], lastRefresh)
			if err != nil {
				return fmt.Errorf("failed to create account %d: %v", i+1, err)
			}
			GlobalPool.accounts = append(GlobalPool.accounts, account)
			GlobalPool.availableCh <- account
			LogInfo("Account %d created successfully", i+1)

			if i < size-1 {
				time.Sleep(time.Duration(1000+rand.Intn(1000)) * time.Millisecond)
			}
		}

		if err := SaveAccounts(GlobalPool.accounts, lastRefresh); err != nil {
			LogWarn("Failed to save accounts: %v", err)
		}
	} else {
		LogInfo("Restoring %d accounts from file (last_refresh: %v)", len(poolData.Accounts), poolData.LastRefresh)
		for _, data := range poolData.Accounts {
			account, err := RestoreAccount(data, poolData.LastRefresh)
			if err != nil {
				LogWarn("Failed to restore account %d: %v", data.ID, err)
				continue
			}
			GlobalPool.accounts = append(GlobalPool.accounts, account)
			GlobalPool.availableCh <- account
			LogInfo("Account %d restored", account.ID)
		}

		if len(GlobalPool.accounts) < size {
			LogWarn("Only restored %d accounts, creating %d new accounts", len(GlobalPool.accounts), size-len(GlobalPool.accounts))

			missingCount := size - len(GlobalPool.accounts)
			umidTokens, err := GenerateBatchUMIDTokens(missingCount)
			if err != nil {
				return fmt.Errorf("failed to generate UMID tokens: %v", err)
			}

			for i := 0; i < missingCount; i++ {
				LogInfo("Creating missing account %d/%d...", i+1, missingCount)
				account, err := createAccountWithUmid(len(GlobalPool.accounts)+1, umidTokens[i], lastRefresh)
				if err != nil {
					return fmt.Errorf("failed to create account %d: %v", len(GlobalPool.accounts)+1, err)
				}
				GlobalPool.accounts = append(GlobalPool.accounts, account)
				GlobalPool.availableCh <- account
				LogInfo("Missing account %d created successfully", i+1)

				if i < missingCount-1 {
					time.Sleep(time.Duration(1000+rand.Intn(1000)) * time.Millisecond)
				}
			}

			if err := SaveAccounts(GlobalPool.accounts, lastRefresh); err != nil {
				LogWarn("Failed to save accounts: %v", err)
			}
		}
	}

	go startRefreshTimer(poolData)

	LogInfo("Account pool initialized successfully: %d accounts", len(GlobalPool.accounts))
	return nil
}

func GuestPoolAccountCount() int {
	if GlobalPool == nil {
		return 0
	}
	GlobalPool.mu.Lock()
	defer GlobalPool.mu.Unlock()
	return len(GlobalPool.accounts)
}

func createAccountWithUmid(id int, umidToken string, lastRefresh time.Time) (*Account, error) {
	account := &Account{
		ID:          id,
		BxUmidtoken: umidToken,
		LastRefresh: lastRefresh,
	}

	if err := refreshAccountTokens(account, false); err != nil {
		return nil, err
	}

	return account, nil
}

func (p *AccountPool) AcquireAccount() (*Account, error) {
	select {
	case account := <-p.availableCh:
		account.mu.Lock()
		account.InUse = true
		account.mu.Unlock()
		LogDebug("Account %d acquired (tokens: %d)", account.ID, len(account.EoCltBacsft))
		return account, nil
	default:
		return nil, fmt.Errorf("no available account")
	}
}

func (p *AccountPool) ReleaseAccount(account *Account) {
	account.mu.Lock()
	account.InUse = false
	account.mu.Unlock()
	LogDebug("Account %d released", account.ID)
	p.availableCh <- account
}

func (a *Account) ConsumeBacsft() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.EoCltBacsft) == 0 {
		return "", fmt.Errorf("no bacsft tokens available")
	}

	token := a.EoCltBacsft[0]
	a.EoCltBacsft = a.EoCltBacsft[1:]

	LogDebug("Account %d: bacsft consumed, remaining: %d", a.ID, len(a.EoCltBacsft))

	if len(a.EoCltBacsft) <= 5 {
		go a.refreshTokens()
	}

	return token, nil
}

func (a *Account) refreshTokens() {
	LogInfo("Account %d: refreshing tokens (bacsft exhausted)", a.ID)

	if err := refreshAccountTokens(a, false); err != nil {
		LogError("Account %d: failed to refresh tokens: %v", a.ID, err)
		return
	}

	a.mu.Lock()
	tokenCount := len(a.EoCltBacsft)
	a.mu.Unlock()

	LogInfo("Account %d: tokens refreshed successfully, bacsft count: %d", a.ID, tokenCount)
}

func startRefreshTimer(poolData *PoolData) {
	var nextRefresh time.Time

	if poolData != nil {
		nextRefresh = poolData.LastRefresh.Add(Cfg.RefreshPeriod)
	} else {
		nextRefresh = time.Now().Add(Cfg.RefreshPeriod)
	}

	if time.Now().After(nextRefresh) {
		LogInfo("Accounts need immediate refresh")
		refreshAllAccounts()
		nextRefresh = time.Now().Add(Cfg.RefreshPeriod)
	}

	waitDuration := time.Until(nextRefresh)
	LogInfo("Next refresh scheduled in %v", waitDuration)

	ticker := time.NewTicker(Cfg.RefreshPeriod)
	defer ticker.Stop()

	time.Sleep(waitDuration)
	refreshAllAccounts()

	for range ticker.C {
		refreshAllAccounts()
	}
}

func refreshAllAccounts() {
	LogInfo("Starting scheduled token refresh for all accounts...")

	LogInfo("Generating %d new UMID tokens in batch...", len(GlobalPool.accounts))
	umidTokens, err := GenerateBatchUMIDTokens(len(GlobalPool.accounts))
	if err != nil {
		LogError("Failed to generate UMID tokens for scheduled refresh: %v", err)
		return
	}

	lastRefresh := time.Now()

	for i, account := range GlobalPool.accounts {
		LogInfo("Refreshing account %d/%d...", i+1, len(GlobalPool.accounts))

		account.mu.Lock()
		account.BxUmidtoken = umidTokens[i]
		account.LastRefresh = lastRefresh
		account.mu.Unlock()

		if err := refreshAccountTokens(account, false); err != nil {
			LogError("Account %d: failed to refresh: %v", account.ID, err)
		} else {
			LogInfo("Account %d: scheduled refresh completed, bacsft count: %d", account.ID, len(account.EoCltBacsft))
		}

		if i < len(GlobalPool.accounts)-1 {
			time.Sleep(time.Duration(1000+rand.Intn(1000)) * time.Millisecond)
		}
	}

	if err := SaveAccounts(GlobalPool.accounts, lastRefresh); err != nil {
		LogWarn("Failed to save accounts after scheduled refresh: %v", err)
	}

	LogInfo("Scheduled refresh completed for all accounts")
}
