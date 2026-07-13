package internal

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Host              string
	Port              string
	PoolSize          int
	AuthKey           string
	AdminKey          string
	LogLevel          string
	RefreshPeriod     time.Duration
	DataDir           string
	DatabasePath      string
	PublicBaseURL     string
	DefaultChatModel  string
	DefaultImageModel string
	DefaultVideoModel string
	BrowserHeadless   bool
	NoVNCURL          string
}

var Cfg *Config

func LoadConfig() {
	godotenv.Load()

	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	poolSizeStr := os.Getenv("POOL_SIZE")
	poolSize := 0
	if poolSizeStr != "" {
		if ps, err := strconv.Atoi(poolSizeStr); err == nil && ps >= 0 {
			poolSize = ps
		}
	}

	authKey := os.Getenv("AUTH_KEY")
	adminKey := os.Getenv("ADMIN_KEY")
	if adminKey == "" {
		adminKey = authKey
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	refreshHoursStr := os.Getenv("REFRESH_HOURS")
	refreshHours := 10
	if refreshHoursStr != "" {
		if rh, err := strconv.Atoi(refreshHoursStr); err == nil && rh > 0 {
			refreshHours = rh
		}
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	databasePath := os.Getenv("DATABASE_PATH")
	if databasePath == "" {
		databasePath = dataDir + "/qianwen-creator-01.sqlite"
	}

	publicBaseURL := strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/")

	defaultChatModel := os.Getenv("DEFAULT_CHAT_MODEL")
	if defaultChatModel == "" {
		defaultChatModel = "qianwen-creator-session"
	}
	defaultImageModel := os.Getenv("DEFAULT_IMAGE_MODEL")
	if defaultImageModel == "" {
		defaultImageModel = "qianwen-creator-wan27-image"
	}
	defaultVideoModel := os.Getenv("DEFAULT_VIDEO_MODEL")
	if defaultVideoModel == "" {
		defaultVideoModel = QianwenVideoModelID
	}
	browserHeadless := strings.ToLower(strings.TrimSpace(os.Getenv("BROWSER_HEADLESS"))) != "false"
	noVNCURL := strings.TrimSpace(os.Getenv("NOVNC_URL"))

	Cfg = &Config{
		Host:              host,
		Port:              port,
		PoolSize:          poolSize,
		AuthKey:           authKey,
		AdminKey:          adminKey,
		LogLevel:          logLevel,
		RefreshPeriod:     time.Duration(refreshHours) * time.Hour,
		DataDir:           dataDir,
		DatabasePath:      databasePath,
		PublicBaseURL:     publicBaseURL,
		DefaultChatModel:  defaultChatModel,
		DefaultImageModel: defaultImageModel,
		DefaultVideoModel: defaultVideoModel,
		BrowserHeadless:   browserHeadless,
		NoVNCURL:          noVNCURL,
	}
}

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

var (
	logger   *log.Logger
	logLevel LogLevel
)

func InitLogger() {
	logger = log.New(os.Stdout, "", 0)

	levelStr := strings.ToLower(Cfg.LogLevel)
	switch levelStr {
	case "debug":
		logLevel = DEBUG
	case "info":
		logLevel = INFO
	case "warn":
		logLevel = WARN
	case "error":
		logLevel = ERROR
	default:
		logLevel = INFO
	}
}

func formatLog(level string, format string, v ...interface{}) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, v...)
	return fmt.Sprintf("[%s] [%s] %s", timestamp, level, message)
}

func LogDebug(format string, v ...interface{}) {
	if logLevel <= DEBUG {
		logger.Println(formatLog("DEBUG", format, v...))
	}
}

func LogInfo(format string, v ...interface{}) {
	if logLevel <= INFO {
		logger.Println(formatLog("INFO", format, v...))
	}
}

func LogWarn(format string, v ...interface{}) {
	if logLevel <= WARN {
		logger.Println(formatLog("WARN", format, v...))
	}
}

func LogError(format string, v ...interface{}) {
	if logLevel <= ERROR {
		logger.Println(formatLog("ERROR", format, v...))
	}
}
