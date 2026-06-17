package internal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

func HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if !requireAPIAuth(w, r) {
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Model == "" {
		req.Model = "tongyi-qwen3-max-model"
	}

	if AppStore != nil {
		if loginAccount, err := AppStore.SelectRunnableAccountForCapability("chat"); err == nil {
			handleLoginChatRequest(w, r, &req, loginAccount)
			return
		}
	}

	if GlobalPool == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "guest_pool_disabled", "Guest chat pool is not initialized. Set POOL_SIZE>0 or add a login chat adapter.")
		return
	}

	account, err := GlobalPool.AcquireAccount()
	if err != nil {
		LogWarn("No available account: %v", err)
		writeAPIError(w, http.StatusTooManyRequests, "no_available_guest_account", "No available guest account in the qianwen pool.")
		return
	}
	defer GlobalPool.ReleaseAccount(account)

	if req.Stream {
		handleStreamRequest(w, r, &req, account)
	} else {
		handleNonStreamRequest(w, r, &req, account)
	}
}

func handleLoginChatRequest(w http.ResponseWriter, r *http.Request, req *ChatRequest, account *AccountRecord) {
	client, err := newQwenWebClient(*account)
	if err != nil {
		_ = AppStore.UpdateAccountStatus(account.ID, "invalid", err.Error(), false)
		writeAPIError(w, http.StatusFailedDependency, "login_account_invalid", err.Error())
		return
	}
	text, _, err := client.chat(r.Context(), req)
	if err != nil {
		_ = AppStore.UpdateAccountStatus(account.ID, "unknown", err.Error(), false)
		writeAPIError(w, http.StatusBadGateway, "qianwen_chat_failed", err.Error())
		return
	}
	_ = AppStore.UpdateAccountStatus(account.ID, "valid", "", true)

	if req.Stream {
		writeLoginChatStream(w, req, text)
		return
	}

	stopReason := "stop"
	response := ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:29]),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{{
			Index: 0,
			Message: &MessageResp{
				Role:    "assistant",
				Content: text,
			},
			FinishReason: &stopReason,
		}},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func writeLoginChatStream(w http.ResponseWriter, req *ChatRequest, text string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}
	completionID := fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:29])
	chunk := ChatCompletionChunk{
		ID:      completionID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{{
			Index: 0,
			Delta: Delta{Content: text},
		}},
	}
	data, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	stopReason := "stop"
	finalChunk := ChatCompletionChunk{
		ID:      completionID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{{
			Index:        0,
			Delta:        Delta{},
			FinishReason: &stopReason,
		}},
	}
	data, _ = json.Marshal(finalChunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func handleStreamRequest(w http.ResponseWriter, r *http.Request, req *ChatRequest, account *Account) {
	resp, err := sendUpstreamRequest(req, account)
	if err != nil {
		LogError("Failed to send upstream request: %v", err)
		http.Error(w, "Upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		LogError("Upstream error: status=%d, body=%s", resp.StatusCode, string(body))
		http.Error(w, "Upstream error", resp.StatusCode)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	completionID := fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:29])
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var lastTextContent string
	var lastReasoningContent string

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}

		var qwResp QianwenResponse
		if err := json.Unmarshal([]byte(payload), &qwResp); err != nil {
			continue
		}

		for _, content := range qwResp.Contents {
			if content.ContentType == "think" {
				var thinkData struct {
					Content       string `json:"content"`
					InferenceCost int    `json:"inferenceCost"`
				}

				if err := json.Unmarshal([]byte(content.Content), &thinkData); err == nil && thinkData.Content != "" {
					var deltaReasoning string

					if content.Incremental {
						deltaReasoning = thinkData.Content
						lastReasoningContent += deltaReasoning
					} else {
						fullReasoning := thinkData.Content
						if len(fullReasoning) > len(lastReasoningContent) {
							deltaReasoning = fullReasoning[len(lastReasoningContent):]
							lastReasoningContent = fullReasoning
						}
					}

					if deltaReasoning != "" {
						chunk := ChatCompletionChunk{
							ID:      completionID,
							Object:  "chat.completion.chunk",
							Created: time.Now().Unix(),
							Model:   req.Model,
							Choices: []Choice{{
								Index:        0,
								Delta:        Delta{ReasoningContent: deltaReasoning},
								FinishReason: nil,
							}},
						}

						data, _ := json.Marshal(chunk)
						fmt.Fprintf(w, "data: %s\n\n", data)
						flusher.Flush()
					}
				}
			} else if content.ContentType == "text" {
				var deltaText string

				if content.Incremental {
					deltaText = content.Content
					lastTextContent += deltaText
				} else {
					fullText := content.Content
					if len(fullText) > len(lastTextContent) {
						deltaText = fullText[len(lastTextContent):]
						lastTextContent = fullText
					}
				}

				if deltaText != "" {
					chunk := ChatCompletionChunk{
						ID:      completionID,
						Object:  "chat.completion.chunk",
						Created: time.Now().Unix(),
						Model:   req.Model,
						Choices: []Choice{{
							Index:        0,
							Delta:        Delta{Content: deltaText},
							FinishReason: nil,
						}},
					}

					data, _ := json.Marshal(chunk)
					fmt.Fprintf(w, "data: %s\n\n", data)
					flusher.Flush()
				}
			}
		}

		if qwResp.MsgStatus == "finished" || qwResp.StopReason == "stop" {
			break
		}
	}

	stopReason := "stop"
	finalChunk := ChatCompletionChunk{
		ID:      completionID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{{
			Index:        0,
			Delta:        Delta{},
			FinishReason: &stopReason,
		}},
	}

	data, _ := json.Marshal(finalChunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func handleNonStreamRequest(w http.ResponseWriter, r *http.Request, req *ChatRequest, account *Account) {
	resp, err := sendUpstreamRequest(req, account)
	if err != nil {
		LogError("Failed to send upstream request: %v", err)
		http.Error(w, "Upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		LogError("Upstream error: status=%d, body=%s", resp.StatusCode, string(body))
		http.Error(w, "Upstream error", resp.StatusCode)
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var fullContent string
	var fullReasoning string

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}

		var qwResp QianwenResponse
		if err := json.Unmarshal([]byte(payload), &qwResp); err != nil {
			continue
		}

		for _, content := range qwResp.Contents {
			if content.ContentType == "think" {
				var thinkData struct {
					Content       string `json:"content"`
					InferenceCost int    `json:"inferenceCost"`
				}
				if err := json.Unmarshal([]byte(content.Content), &thinkData); err == nil && thinkData.Content != "" {
					if content.Incremental {
						fullReasoning += thinkData.Content
					} else {
						fullReasoning = thinkData.Content
					}
				}
			} else if content.ContentType == "text" {
				if content.Incremental {
					fullContent += content.Content
				} else {
					fullContent = content.Content
				}
			}
		}

		if qwResp.MsgStatus == "finished" || qwResp.StopReason == "stop" {
			break
		}
	}

	completionID := fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:29])
	stopReason := "stop"

	response := ChatCompletionResponse{
		ID:      completionID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{{
			Index: 0,
			Message: &MessageResp{
				Role:             "assistant",
				Content:          fullContent,
				ReasoningContent: fullReasoning,
			},
			FinishReason: &stopReason,
		}},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func sendUpstreamRequest(req *ChatRequest, account *Account) (*http.Response, error) {
	tParam := generateRandomHex(11)
	reqt := time.Now().UnixMilli()

	sacsft, err := account.ConsumeBacsft()
	if err != nil {
		return nil, fmt.Errorf("failed to get bacsft: %v", err)
	}

	ve := "1.0.0"
	kp := ""
	signature := GenerateSignature(account.EoCltDvidn, ve, kp, tParam, sacsft, reqt)

	var contents []QianwenContent
	for _, msg := range req.Messages {
		content := msg.Content

		if msg.Role == "system" {
			content = "SYSTEM: " + content
		} else if msg.Role == "assistant" {
			content = "ASSISTANT: " + content
		}

		contents = append(contents, QianwenContent{
			Content:     content,
			ContentType: "text",
			Role:        msg.Role,
		})
	}

	payload := map[string]interface{}{
		"sessionId":    "",
		"sessionType":  "text_chat",
		"parentMsgId":  "",
		"model":        "",
		"mode":         "chat",
		"userAction":   "",
		"actionSource": "",
		"contents":     contents,
		"action":       "next",
		"requestId":    uuid.New().String(),
		"params": map[string]interface{}{
			"specifiedModel":   req.Model,
			"lastUseModelList": []string{req.Model},
			"recordModelName":  req.Model,
			"searchType":       "off",
			"bizSceneInfo":     map[string]interface{}{},
		},
		"topicId": uuid.New().String(),
	}

	bodyBytes, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/dialog/guest/conversation/v2?t=%s", account.Client.apiURL, tParam)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Origin", account.Client.baseURL)
	httpReq.Header.Set("Referer", account.Client.baseURL+"/chat")
	httpReq.Header.Set("x-xsrf-token", account.XsrfToken)
	httpReq.Header.Set("x-deviceid", account.DeviceID)
	httpReq.Header.Set("x-platform", "pc_tongyi")
	httpReq.Header.Set("clt-acs-sign", signature)
	httpReq.Header.Set("clt-acs-reqt", fmt.Sprintf("%d", reqt))
	httpReq.Header.Set("clt-acs-request-params", "t")
	httpReq.Header.Set("clt-acs-caer", "vrad")
	httpReq.Header.Set("eo-clt-dvidn", account.EoCltDvidn)
	httpReq.Header.Set("eo-clt-sacsft", sacsft)
	httpReq.Header.Set("eo-clt-snver", account.EoCltSnver)
	httpReq.Header.Set("eo-clt-actkn", account.EoCltActkn)
	httpReq.Header.Set("eo-clt-acs-ve", ve)
	httpReq.Header.Set("eo-clt-acs-kp", kp)

	LogDebug("Sending request to: %s", url)

	return account.Client.client.Do(httpReq)
}
