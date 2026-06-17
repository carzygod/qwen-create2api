package internal

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

var (
	browserCtx       context.Context
	browserCancel    context.CancelFunc
	browserCtxCancel context.CancelFunc
	browserMutex     sync.Mutex
	browserOnce      sync.Once
)

func initBrowser() {
	browserOnce.Do(func() {
		LogInfo("Starting browser initialization...")

		baseCtx := context.Background()

		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-dev-shm-usage", true),
			chromedp.Flag("disable-setuid-sandbox", true),
			chromedp.Flag("disable-software-rasterizer", true),
			chromedp.Flag("disable-extensions", true),
			chromedp.Flag("disable-background-networking", true),
			chromedp.Flag("disable-default-apps", true),
			chromedp.Flag("disable-sync", true),
			chromedp.Flag("disable-translate", true),
			chromedp.Flag("hide-scrollbars", true),
			chromedp.Flag("metrics-recording-only", true),
			chromedp.Flag("mute-audio", true),
			chromedp.Flag("no-first-run", true),
			chromedp.Flag("safebrowsing-disable-auto-update", true),
			chromedp.UserAgent(generateRandomUserAgent()),
		)

		if runtime.GOOS != "windows" {
			opts = append(opts, chromedp.Flag("single-process", true))
			LogInfo("Non-Windows system detected, using single-process mode")
		} else {
			LogInfo("Windows system detected, using multi-process mode")
		}

		LogInfo("Creating allocator context...")
		allocCtx, allocCancel := chromedp.NewExecAllocator(baseCtx, opts...)
		browserCancel = allocCancel

		LogInfo("Creating browser context...")
		ctx, ctxCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(format string, args ...interface{}) {
			LogDebug("[chromedp] "+format, args...)
		}))
		browserCtx = ctx
		browserCtxCancel = ctxCancel

		LogInfo("Starting browser process...")
		if err := chromedp.Run(browserCtx); err != nil {
			LogError("Failed to start browser: %v", err)
			return
		}

		LogInfo("Browser instance initialized successfully")
	})
}

func CloseBrowser() {
	if browserCancel != nil {
		if browserCtxCancel != nil {
			browserCtxCancel()
		}
		browserCancel()
		browserCancel = nil
		browserCtxCancel = nil
		browserCtx = nil
		browserOnce = sync.Once{}
		LogInfo("Browser instance closed")
	}
}

func GenerateUMIDToken() (string, error) {
	tokens, err := GenerateBatchUMIDTokens(1)
	if err != nil {
		return "", err
	}
	return tokens[0], nil
}

func GenerateUMIDTokenWithRetry(maxRetries int) (string, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		token, err := GenerateUMIDToken()
		if err == nil {
			return token, nil
		}

		lastErr = err
		LogWarn("Failed to generate UMID token (attempt %d/%d): %v", i+1, maxRetries, err)
		time.Sleep(2 * time.Second)
	}

	return "", fmt.Errorf("failed after %d retries: %v", maxRetries, lastErr)
}

func GenerateBatchUMIDTokens(count int) ([]string, error) {
	browserMutex.Lock()
	defer browserMutex.Unlock()

	LogInfo("Generating %d UMID tokens in batch...", count)
	initBrowser()

	LogInfo("After initBrowser - browserCtx: %v", browserCtx != nil)
	if browserCtx == nil {
		return nil, fmt.Errorf("browserCtx is nil after initialization")
	}

	tokens := make([]string, 0, count)
	htmlContent := generateUMIDHTML()

	for i := 0; i < count; i++ {
		const maxTokenRetries = 3
		tokenGenerated := false
		for retry := 1; retry <= maxTokenRetries; retry++ {
			var token string
			var status string

			LogDebug("Attempting to navigate for token %d/%d (retry %d/%d)...", i+1, count, retry, maxTokenRetries)
			err := chromedp.Run(browserCtx,
				chromedp.Navigate("data:text/html,"+htmlContent),
				chromedp.Sleep(500*time.Millisecond),
			)

			if err != nil {
				LogWarn("Failed to navigate for UMID token %d/%d: %v", i+1, count, err)
				time.Sleep(1 * time.Second)
				continue
			}

			maxAttempts := 20
			attempt := 0
			for attempt = 0; attempt < maxAttempts; attempt++ {
				err = chromedp.Run(browserCtx,
					chromedp.Evaluate(`window.umidStatus`, &status),
				)

				if err != nil {
					break
				}

				if status == "success" {
					err = chromedp.Run(browserCtx,
						chromedp.Evaluate(`window.umidResult ? window.umidResult.tn : null`, &token),
					)
					break
				} else if status == "error" {
					break
				}

				time.Sleep(500 * time.Millisecond)
			}

			if err != nil {
				LogWarn("Failed to generate UMID token %d/%d: %v", i+1, count, err)
				time.Sleep(1 * time.Second)
				continue
			}

			if token == "" || status != "success" {
				LogWarn("Empty or failed UMID token %d/%d (status: %s), retrying...", i+1, count, status)
				time.Sleep(1 * time.Second)
				continue
			}

			tokens = append(tokens, token)
			tokenGenerated = true
			LogInfo("Generated UMID token %d/%d (took ~%dms)", i+1, count, (attempt+1)*500)
			break
		}

		if !tokenGenerated {
			return nil, fmt.Errorf("failed to generate UMID token %d/%d after %d retries", i+1, count, maxTokenRetries)
		}

		if i < count-1 {
			time.Sleep(200 * time.Millisecond)
		}
	}

	CloseBrowser()

	LogInfo("Successfully generated %d UMID tokens", len(tokens))
	return tokens, nil
}

func generateUMIDHTML() string {
	return `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>UMID Generator</title>
</head>
<body>
    <script src="https://g.alicdn.com/AWSC/AWSC/awsc.js"></script>
    <script>
        window.umidResult = null;
        window.umidError = null;
        window.umidStatus = 'loading';

        window.AWSC.use('um', function(state, umModule) {
            if (state === 'loaded') {
                const config = {
                    appName: 'wagbridgead-sm-nginx-quarkpc-security-calibration-time-web',
                    serviceLocation: 'cn',
                    timeout: 10000
                };

                umModule.init(config, function(status, result) {
                    if (status === 'success') {
                        window.umidResult = result;
                        window.umidStatus = 'success';
                    } else {
                        window.umidError = { status, result };
                        window.umidStatus = 'error';
                    }
                });
            } else {
                window.umidError = { state };
                window.umidStatus = 'error';
            }
        });
    </script>
</body>
</html>`
}

func generateRandomUserAgent() string {
	chromeVersions := []string{"120", "121", "122", "123", "124", "125", "126", "127", "128", "129", "130", "131", "142"}
	version := chromeVersions[rand.Intn(len(chromeVersions))]

	return fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36", version)
}
