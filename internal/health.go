package internal

import "net/http"

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	storeReady := AppStore != nil
	guestCount := GuestPoolAccountCount()
	guestReady := Cfg.PoolSize > 0 && guestCount > 0 && GuestPoolInitError == ""
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":               true,
		"service":          QianwenCreatorProviderCode,
		"store_ready":      storeReady,
		"guest_ready":      guestReady,
		"guest_pool_size":  Cfg.PoolSize,
		"guest_pool_count": guestCount,
		"guest_pool_error": GuestPoolInitError,
		"data_dir":         Cfg.DataDir,
		"database":         Cfg.DatabasePath,
	})
}
