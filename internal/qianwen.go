package internal

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/google/uuid"
)

type QianwenClient struct {
	client       *http.Client
	baseURL      string
	apiURL       string
	baAPIURL     string
	deviceID     string
	umDistinctid string
	cna          string
	tfstk        string
	xsrfToken    string
}

func NewQianwenClient() (*QianwenClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		MaxIdleConns:        300,
		MaxIdleConnsPerHost: 150,
		IdleConnTimeout:     30 * time.Second,
	}

	client := &http.Client{
		Jar:       jar,
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	qc := &QianwenClient{
		client:   client,
		baseURL:  "https://www.qianwen.com",
		apiURL:   "https://api.qianwen.com",
		baAPIURL: "https://ext.quark.cn",
		deviceID: uuid.New().String(),
	}

	qc.initCookies()
	return qc, nil
}

func (qc *QianwenClient) initCookies() {
	timestampHex := fmt.Sprintf("%x", time.Now().UnixMilli())
	qc.umDistinctid = fmt.Sprintf("%s-%s-26061b51-240000-%s%s",
		timestampHex,
		generateRandomHex(16),
		timestampHex,
		generateRandomHex(7))

	qc.cna = generateRandomToken(28, "")
	qc.tfstk = generateRandomToken(64, "c")
}

func (qc *QianwenClient) GetXSRFToken() error {
	req, err := http.NewRequest("GET", qc.baseURL+"/chat", nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")

	resp, err := qc.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	LogDebug("XSRF Token request: status=%d", resp.StatusCode)

	for _, cookie := range resp.Cookies() {
		LogDebug("Received cookie: %s=%s", cookie.Name, cookie.Value)
		if cookie.Name == "XSRF-TOKEN" {
			qc.xsrfToken = cookie.Value
			LogDebug("Got XSRF-TOKEN: %s", qc.xsrfToken)
			return nil
		}
	}

	LogWarn("XSRF-TOKEN not found. Status: %d, Cookies count: %d", resp.StatusCode, len(resp.Cookies()))
	return fmt.Errorf("XSRF-TOKEN not found in response")
}

func (qc *QianwenClient) GetGuestTicket() (string, error) {
	reqBody := map[string]interface{}{}
	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", qc.apiURL+"/dialog/guest/init", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", qc.baseURL)
	req.Header.Set("Referer", qc.baseURL+"/")
	req.Header.Set("x-deviceid", qc.deviceID)

	resp, err := qc.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result GuestTicketResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %v, body: %s", err, string(body))
	}

	if !result.Success || result.Data.Ticket == "" {
		return "", fmt.Errorf("failed to get guest ticket: %s", string(body))
	}

	LogDebug("Got guest ticket: %s", result.Data.Ticket)
	return result.Data.Ticket, nil
}

func (qc *QianwenClient) RegisterAndGetTokens(bxUmidtoken string) (*RegisterResponse, error) {
	fingerprint := generateFingerprint(qc.deviceID)
	chid := generateChid()

	payload := map[string]interface{}{
		"screenResolution":    "2048x1152",
		"cookieEnabled":       true,
		"localStorageEnabled": true,
		"timezoneOffset":      "Asia/Shanghai",
		"fontList":            []string{"Arial", "Calibri", "Century", "SimHei"},
		"pluginList":          []interface{}{},
		"language":            []string{"zh-CN"},
		"unifyRelateGenerate": []string{"qwen_business"},
		"fingerprint":         fingerprint,
		"businessScene":       "qwen_web",
		"chid":                chid,
	}

	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", qc.baAPIURL+"/security/external/access/register", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", qc.baseURL)
	req.Header.Set("Referer", qc.baseURL+"/chat")
	req.Header.Set("bx-umidtoken", bxUmidtoken)
	req.Header.Set("clt-acs-caer", "vrad")
	req.Header.Set("eo-clt-sftcnt", "20")

	resp, err := qc.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result RegisterResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v, body: %s", err, string(body))
	}

	if result.Status != 0 {
		return nil, fmt.Errorf("register failed: %s", string(body))
	}

	LogDebug("Register success, got %d bacsft tokens", len(result.Data.EoCltBacsft))
	return &result, nil
}

func generateRandomHex(length int) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, length)
	for i := range result {
		result[i] = hexChars[rand.Intn(len(hexChars))]
	}
	return string(result)
}

func generateRandomToken(length int, prefix string) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"
	result := make([]byte, length-len(prefix))
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return prefix + string(result)
}

func generateFingerprint(deviceID string) string {
	data := fmt.Sprintf("%d%f%s", time.Now().UnixNano(), rand.Float64(), deviceID)
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", hash)
}

func generateChid() string {
	timestampMs := time.Now().UnixMilli()
	randomStr := generateRandomToken(11, "")
	return fmt.Sprintf("%d%s", timestampMs, randomStr)
}
