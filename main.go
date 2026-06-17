package main

import (
	"fmt"
	"net/http"
	"qianwencreator2api/internal"
)

func main() {
	internal.LoadConfig()
	internal.InitLogger()

	internal.LogInfo("===========================================")
	internal.LogInfo("QIANWEN-CREATOR-01 Server Starting...")
	internal.LogInfo("===========================================")
	internal.LogInfo("Host: %s", internal.Cfg.Host)
	internal.LogInfo("Port: %s", internal.Cfg.Port)
	internal.LogInfo("Pool Size: %d", internal.Cfg.PoolSize)
	internal.LogInfo("Log Level: %s", internal.Cfg.LogLevel)
	internal.LogInfo("Data Dir: %s", internal.Cfg.DataDir)
	internal.LogInfo("Database: %s", internal.Cfg.DatabasePath)
	internal.LogInfo("===========================================")

	if err := internal.InitStore(); err != nil {
		internal.LogError("Failed to initialize sqlite store: %v", err)
		return
	}

	if err := internal.InitPool(internal.Cfg.PoolSize); err != nil {
		internal.GuestPoolInitError = err.Error()
		internal.LogError("Failed to initialize guest pool: %v", err)
		internal.LogWarn("Continuing without a ready guest pool. Admin, SQLite, account management, and Creator media task routes remain available.")
	}

	http.HandleFunc("/health", internal.HandleHealth)
	http.HandleFunc("/auth/status", internal.HandleAuthStatus)
	http.HandleFunc("/auth/qr", internal.HandleAuthQR)
	http.HandleFunc("/auth/login", internal.HandleAuthLogin)
	http.HandleFunc("/auth/capture", internal.HandleAuthCapture)
	http.HandleFunc("/admin", internal.HandleAdminPage)
	http.HandleFunc("/api/", internal.HandleAdminAPI)
	http.HandleFunc("/v1/models", internal.HandleModels)
	http.HandleFunc("/v1/chat/completions", internal.HandleChatCompletions)
	http.HandleFunc("/v1/images/generations", internal.HandleImageGenerations)
	http.HandleFunc("/v1/video/generations/sync", internal.HandleVideoGenerationsSync)
	http.HandleFunc("/v1/videos/generations/sync", internal.HandleVideoGenerationsSync)
	http.HandleFunc("/v1/videos", internal.HandleVideoGenerations)
	http.HandleFunc("/v1/videos/", internal.HandleVideoTask)
	http.HandleFunc("/v1/video/generations", internal.HandleVideoGenerations)
	http.HandleFunc("/v1/video/generations/", internal.HandleVideoTask)
	http.HandleFunc("/v1/videos/generations", internal.HandleVideoGenerations)
	http.HandleFunc("/v1/videos/generations/", internal.HandleVideoTask)
	http.HandleFunc("/v1/tasks/", internal.HandleGenericTask)

	addr := fmt.Sprintf("%s:%s", internal.Cfg.Host, internal.Cfg.Port)
	internal.LogInfo("Server listening on %s", addr)
	internal.LogInfo("===========================================")

	if err := http.ListenAndServe(addr, nil); err != nil {
		internal.LogError("Server failed: %v", err)
	}
}
