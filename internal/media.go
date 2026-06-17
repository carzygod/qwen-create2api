package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ImageGenerationRequest struct {
	Model          string      `json:"model"`
	Prompt         string      `json:"prompt"`
	N              int         `json:"n"`
	Size           string      `json:"size"`
	Quality        string      `json:"quality"`
	Style          string      `json:"style"`
	ResponseFormat string      `json:"response_format"`
	AspectRatio    string      `json:"aspect_ratio"`
	Resolution     string      `json:"resolution"`
	Seed           interface{} `json:"seed"`
	NegativePrompt string      `json:"negative_prompt"`
	ReferenceImage string      `json:"reference_image"`
}

type VideoGenerationRequest struct {
	Model                string      `json:"model"`
	Prompt               string      `json:"prompt"`
	Duration             int         `json:"duration"`
	DurationSeconds      int         `json:"duration_seconds"`
	Seconds              int         `json:"seconds"`
	Ratio                string      `json:"ratio"`
	Size                 string      `json:"size"`
	AspectRatio          string      `json:"aspect_ratio"`
	Resolution           string      `json:"resolution"`
	ImageURL             string      `json:"image_url"`
	Image                string      `json:"image"`
	FileID               string      `json:"file_id"`
	FirstFrameImage      string      `json:"first_frame_image"`
	LastFrameImage       string      `json:"last_frame_image"`
	FirstFrameMaterialID string      `json:"first_frame_material_id"`
	LastFrameMaterialID  string      `json:"last_frame_material_id"`
	ReferenceMaterialIDs []string    `json:"reference_material_ids"`
	ReferenceImages      []string    `json:"reference_images"`
	Seed                 interface{} `json:"seed"`
	NegativePrompt       string      `json:"negative_prompt"`
	Metadata             interface{} `json:"metadata"`
	AccountID            string      `json:"account_id"`
	Async                *bool       `json:"async"`
	Wait                 bool        `json:"wait"`
	Sync                 bool        `json:"sync"`
	Blocking             bool        `json:"blocking"`
}

func HandleImageGenerations(w http.ResponseWriter, r *http.Request) {
	if !requireAPIAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is supported.")
		return
	}
	var req ImageGenerationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeAPIError(w, http.StatusBadRequest, "prompt_required", "prompt is required.")
		return
	}
	if req.Model == "" {
		req.Model = Cfg.DefaultImageModel
	}
	if req.N == 0 {
		req.N = 1
	}

	account, err := AppStore.SelectRunnableAccountForCapability("image")
	if err != nil {
		if err == sql.ErrNoRows {
			writeAPIError(w, http.StatusFailedDependency, "no_image_account", "No image-capable Qianwen Creator login account is available. Add an account in Admin first.")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "account_select_failed", err.Error())
		return
	}

	client, err := newQwenWebClient(*account)
	if err != nil {
		writeAPIError(w, http.StatusFailedDependency, "login_account_invalid", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	state, events, err := client.submitImage(ctx, req)
	if err != nil {
		_ = AppStore.UpdateAccountStatus(account.ID, "unknown", err.Error(), false)
		writeAPIError(w, http.StatusBadGateway, "qianwen_image_submit_failed", err.Error())
		return
	}
	urls := filterMediaURLs(extractURLs(events), "image")
	if len(urls) == 0 {
		result, err := client.pollMedia(ctx, state, "image", 130*time.Second)
		if err != nil {
			_ = AppStore.UpdateAccountStatus(account.ID, "unknown", err.Error(), false)
			writeAPIError(w, http.StatusGatewayTimeout, "qianwen_image_poll_failed", err.Error())
			return
		}
		urls = result.URLs
	}
	urls = limitURLs(urls, req.N)
	if len(urls) == 0 {
		writeAPIError(w, http.StatusBadGateway, "qianwen_image_empty_result", "Qianwen image generation completed without a media URL.")
		return
	}
	_ = AppStore.UpdateAccountStatus(account.ID, "valid", "", true)
	data := make([]map[string]string, 0, len(urls))
	for _, url := range urls {
		data = append(data, map[string]string{"url": url})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"created": time.Now().Unix(),
		"data":    data,
	})
}

func HandleVideoGenerations(w http.ResponseWriter, r *http.Request) {
	if !requireAPIAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is supported.")
		return
	}
	var req VideoGenerationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeAPIError(w, http.StatusBadRequest, "prompt_required", "prompt is required.")
		return
	}
	if req.Model == "" {
		req.Model = Cfg.DefaultVideoModel
	}
	normalizeVideoRequest(&req)

	if wantsSyncVideo(req) {
		handleVideoGenerationsSyncRequest(w, r, req)
		return
	}

	if _, err := videoCandidateAccounts(req); err != nil {
		writeNoVideoAccountError(w, err)
		return
	}

	body, _ := json.Marshal(req)
	task := &TaskRecord{
		Type:        "video",
		Status:      "queued",
		Model:       req.Model,
		RequestJSON: string(body),
	}
	if err := AppStore.CreateTask(task); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "task_create_failed", err.Error())
		return
	}
	go executeVideoTask(context.Background(), task.ID, req)
	fresh, _ := AppStore.GetTask(task.ID)
	writeJSON(w, http.StatusOK, formatVideoTaskResponse(r, freshOrOriginal(fresh, task)))
}

func HandleVideoGenerationsSync(w http.ResponseWriter, r *http.Request) {
	if !requireAPIAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is supported.")
		return
	}
	var req VideoGenerationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeAPIError(w, http.StatusBadRequest, "prompt_required", "prompt is required.")
		return
	}
	if req.Model == "" {
		req.Model = Cfg.DefaultVideoModel
	}
	normalizeVideoRequest(&req)
	handleVideoGenerationsSyncRequest(w, r, req)
}

func handleVideoGenerationsSyncRequest(w http.ResponseWriter, r *http.Request, req VideoGenerationRequest) {
	body, _ := json.Marshal(req)
	task := &TaskRecord{
		Type:        "video",
		Status:      "queued",
		Model:       req.Model,
		RequestJSON: string(body),
	}
	if err := AppStore.CreateTask(task); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "task_create_failed", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 9*time.Minute)
	defer cancel()
	if err := executeVideoTask(ctx, task.ID, req); err != nil {
		LogWarn("Synchronous video generation task %s failed: %v", task.ID, err)
	}
	fresh, err := AppStore.GetTask(task.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "task_get_failed", err.Error())
		return
	}
	status := http.StatusOK
	if fresh.Status == "failed" {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, formatVideoTaskResponse(r, fresh))
}

func executeVideoTask(ctx context.Context, taskID string, req VideoGenerationRequest) error {
	accounts, err := videoCandidateAccounts(req)
	if err != nil {
		message := noVideoAccountMessage(err)
		_ = AppStore.UpdateTaskFailed(taskID, "no_video_account", message, "")
		return fmt.Errorf("%s", message)
	}

	var attemptErrors []string
	for _, account := range accounts {
		if cancelled, _ := AppStore.IsTaskCancelled(taskID); cancelled {
			return nil
		}
		err := submitAndPollVideoWithAccount(ctx, taskID, req, account)
		if err == nil {
			return nil
		}
		attemptErrors = append(attemptErrors, fmt.Sprintf("%s: %s", account.ID, err.Error()))
		_ = AppStore.UpdateAccountStatus(account.ID, "unknown", err.Error(), false)
		if strings.TrimSpace(req.AccountID) != "" {
			break
		}
		LogWarn("Qianwen Creator video account %s failed, trying next account if available: %v", account.ID, err)
	}
	message := strings.Join(attemptErrors, "; ")
	if message == "" {
		message = "No video-capable Qianwen Creator login account is available."
	}
	_ = AppStore.UpdateTaskFailed(taskID, "qianwen_creator_video_generation_failed", message, "")
	return fmt.Errorf("%s", message)
}

func writeNoVideoAccountError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "account_select_failed"
	message := err.Error()
	if err == sql.ErrNoRows {
		status = http.StatusFailedDependency
		code = "no_video_account"
		message = noVideoAccountMessage(err)
	}
	writeAPIError(w, status, code, message)
}

func noVideoAccountMessage(err error) string {
	if err == sql.ErrNoRows {
		return "No video-capable Qianwen Creator login account is available. Add an account in Admin and run account test until status is valid."
	}
	return err.Error()
}

func submitAndPollVideoWithAccount(ctx context.Context, taskID string, req VideoGenerationRequest, account AccountRecord) error {
	client, err := newQwenWebClient(account)
	if err != nil {
		return fmt.Errorf("login_account_invalid: %w", err)
	}
	submitCtx, cancelSubmit := context.WithTimeout(ctx, 90*time.Second)
	state, events, err := client.submitVideo(submitCtx, req)
	cancelSubmit()
	if err != nil {
		return fmt.Errorf("qianwen_creator_video_submit_failed: %w", err)
	}
	_ = AppStore.UpdateTaskRunningWithAccount(taskID, account.ID, state.ReqID, state.SessionID, marshalCompact(state), marshalCompact(events))

	pollCtx, cancelPoll := context.WithTimeout(ctx, 8*time.Minute)
	defer cancelPoll()
	result, err := client.pollMedia(pollCtx, state, "video", 7*time.Minute)
	if err != nil {
		_ = AppStore.UpdateTaskFailed(taskID, "qianwen_creator_video_poll_failed", err.Error(), marshalCompact(result))
		return fmt.Errorf("qianwen_creator_video_poll_failed: %w", err)
	}
	if len(result.URLs) == 0 {
		return fmt.Errorf("qianwen creator video generation completed without a media URL")
	}
	data := make([]map[string]interface{}, 0, len(result.URLs))
	for _, url := range result.URLs {
		data = append(data, map[string]interface{}{
			"url":       url,
			"video_url": url,
		})
	}
	_ = AppStore.UpdateTaskCompleted(taskID, marshalCompact(map[string]interface{}{
		"data": data,
		"urls": result.URLs,
	}), marshalCompact(result.Events))
	_ = AppStore.UpdateAccountStatus(account.ID, "valid", "", true)
	return nil
}

func HandleVideoTask(w http.ResponseWriter, r *http.Request) {
	if !requireAPIAuth(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/video/generations/")
	id = strings.TrimPrefix(id, "/v1/videos/generations/")
	id = strings.TrimPrefix(id, "/v1/videos/")
	id = strings.Trim(id, "/")
	if strings.HasSuffix(id, "/cancel") {
		id = strings.TrimSuffix(id, "/cancel")
		id = strings.Trim(id, "/")
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is supported.")
			return
		}
		if id == "" {
			writeAPIError(w, http.StatusNotFound, "task_id_required", "Task id is required.")
			return
		}
		if err := AppStore.CancelTask(id); err != nil {
			if err == sql.ErrNoRows {
				writeAPIError(w, http.StatusNotFound, "task_not_found", "Task not found.")
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "task_cancel_failed", err.Error())
			return
		}
		task, _ := AppStore.GetTask(id)
		writeJSON(w, http.StatusOK, formatVideoTaskResponse(r, task))
		return
	}
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is supported.")
		return
	}
	if id == "" {
		writeAPIError(w, http.StatusNotFound, "task_id_required", "Task id is required.")
		return
	}
	task, err := AppStore.GetTask(id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeAPIError(w, http.StatusNotFound, "task_not_found", "Task not found.")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "task_get_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, formatVideoTaskResponse(r, task))
}

func HandleGenericTask(w http.ResponseWriter, r *http.Request) {
	if !requireAPIAuth(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/tasks/")
	id = strings.Trim(id, "/")
	task, err := AppStore.GetTask(id)
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

func formatVideoTaskResponse(r *http.Request, task *TaskRecord) map[string]interface{} {
	if isOpenAIVideoPath(r.URL.Path) {
		return normalizeOpenAIVideoTaskResponse(task)
	}
	return normalizeVideoTaskResponse(task)
}

func isOpenAIVideoPath(path string) bool {
	return path == "/v1/videos" || (strings.HasPrefix(path, "/v1/videos/") && !strings.HasPrefix(path, "/v1/videos/generations"))
}

func createProtocolRequiredTask(taskType, model, accountID string, req interface{}, code, message string) (*TaskRecord, error) {
	body, _ := json.Marshal(req)
	task := &TaskRecord{
		Type:              taskType,
		Status:            "failed",
		Model:             model,
		ProviderAccountID: accountID,
		RequestJSON:       string(body),
		ErrorCode:         code,
		ErrorMessage:      message,
		CompletedAt:       nowISO(),
	}
	if err := AppStore.CreateTask(task); err != nil {
		return nil, err
	}
	return task, nil
}

func normalizeVideoTaskResponse(task *TaskRecord) map[string]interface{} {
	if task == nil {
		return map[string]interface{}{}
	}
	status := normalizeTaskStatus(task.Status)
	resp := map[string]interface{}{
		"id":         task.ID,
		"task_id":    task.ID,
		"object":     "video.generation.task",
		"created":    parseUnix(task.CreatedAt),
		"updated":    parseUnix(task.UpdatedAt),
		"model":      task.Model,
		"provider":   QianwenCreatorProviderCode,
		"status":     status,
		"poll_url":   "/v1/video/generations/" + task.ID,
		"account_id": task.ProviderAccountID,
	}
	if providerModel := taskProviderModel(task); providerModel != "" {
		resp["provider_model"] = providerModel
	}
	if req := taskRequestMap(task); req != nil {
		if v, ok := req["duration"]; ok {
			resp["duration"] = v
		}
		if v, ok := req["ratio"]; ok {
			resp["ratio"] = v
		} else if v, ok := req["aspect_ratio"]; ok {
			resp["ratio"] = v
		}
	}
	if task.ResultJSON != "" {
		var result map[string]interface{}
		if json.Unmarshal([]byte(task.ResultJSON), &result) == nil {
			if data, ok := result["data"]; ok {
				resp["data"] = data
				resp["output"] = data
				resp["result"] = map[string]interface{}{"data": data}
				if firstURL := firstVideoURL(data); firstURL != "" {
					resp["url"] = firstURL
					resp["video_url"] = firstURL
				}
			} else {
				resp["data"] = result
			}
		}
	}
	if task.ErrorCode != "" || task.ErrorMessage != "" {
		resp["error"] = ErrorDetail{
			Code:    task.ErrorCode,
			Message: task.ErrorMessage,
			Type:    "qianwen_creator_error",
		}
	}
	return resp
}

func normalizeOpenAIVideoTaskResponse(task *TaskRecord) map[string]interface{} {
	resp := normalizeVideoTaskResponse(task)
	if len(resp) == 0 {
		return resp
	}
	status, _ := resp["status"].(string)
	openAIStatus := normalizeOpenAIVideoStatus(status)
	resp["object"] = "video"
	resp["status"] = openAIStatus
	resp["progress"] = openAIVideoProgress(openAIStatus)
	resp["created_at"] = resp["created"]
	delete(resp, "created")
	if updated, ok := resp["updated"]; ok {
		resp["completed_at"] = updated
		delete(resp, "updated")
	}
	if duration, ok := resp["duration"]; ok {
		resp["seconds"] = fmt.Sprintf("%v", duration)
	}
	if ratio, ok := resp["ratio"]; ok {
		resp["size"] = fmt.Sprintf("%v", ratio)
	}
	if url, ok := resp["url"].(string); ok && url != "" {
		resp["metadata"] = map[string]interface{}{"url": url}
	}
	if _, ok := resp["error"]; ok {
		resp["error"] = map[string]interface{}{
			"message": fmt.Sprintf("%v", resp["error"]),
			"code":    "qianwen_creator_error",
		}
	}
	return resp
}

func normalizeVideoRequest(req *VideoGenerationRequest) {
	req.Model = normalizeQianwenVideoModel(req.Model)
	if req.Duration == 0 {
		switch {
		case req.DurationSeconds > 0:
			req.Duration = req.DurationSeconds
		case req.Seconds > 0:
			req.Duration = req.Seconds
		default:
			req.Duration = 10
		}
	}
	if req.AspectRatio == "" {
		if req.Ratio != "" {
			req.AspectRatio = req.Ratio
		} else if req.Size != "" {
			req.AspectRatio = req.Size
		}
	}
	if strings.Contains(req.AspectRatio, "x") {
		req.AspectRatio = strings.ReplaceAll(req.AspectRatio, "x", ":")
	}
	if req.AspectRatio == "" {
		req.AspectRatio = "16:9"
	}
	req.Ratio = req.AspectRatio
	if req.Resolution == "" {
		req.Resolution = "720P"
	}
	if req.FirstFrameImage == "" {
		if req.ImageURL != "" {
			req.FirstFrameImage = req.ImageURL
		} else if req.Image != "" {
			req.FirstFrameImage = req.Image
		} else if req.FileID != "" {
			req.FirstFrameImage = req.FileID
		}
	}
	if req.FirstFrameMaterialID == "" && looksLikeMaterialID(req.FirstFrameImage) {
		req.FirstFrameMaterialID = req.FirstFrameImage
	}
	if req.LastFrameMaterialID == "" && looksLikeMaterialID(req.LastFrameImage) {
		req.LastFrameMaterialID = req.LastFrameImage
	}
}

func normalizeQianwenVideoModel(model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return Cfg.DefaultVideoModel
	}
	compact := strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(trimmed))
	switch compact {
	case "wan21frame", "wanx21kf2vplus", "wan21kf2vplus", "qianwencreatorwan21frame":
		return "qianwen-creator-wan21-frame"
	case "wan22flashframe", "wan22kf2vflash", "wan22flashkf2v", "qianwencreatorwan22flashframe":
		return "qianwen-creator-wan22-flash-frame"
	case "wan25i2v", "wan25firstframe", "wan25i2vpreview", "qianwencreatorwan25i2v":
		return "qianwen-creator-wan25-i2v"
	case "wan25t2v", "wan25textvideo", "wan25t2vpreview", "qianwencreatorwan25t2v":
		return "qianwen-creator-wan25-t2v"
	case "wan27frame", "wan27i2v", "qianwencreatorwan27frame":
		return "qianwen-creator-wan27-frame"
	case "happyhorse", "happyhorse10", "happyhorse1.0", "happyhorsei2v", "qianwencreatorhappyhorsei2v":
		return "qianwen-creator-happyhorse-i2v"
	default:
		return trimmed
	}
}

func looksLikeMaterialID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	return !strings.HasPrefix(lower, "http://") &&
		!strings.HasPrefix(lower, "https://") &&
		!strings.HasPrefix(lower, "data:") &&
		!strings.Contains(value, "/") &&
		len(value) >= 8
}

func wantsSyncVideo(req VideoGenerationRequest) bool {
	if req.Async != nil && !*req.Async {
		return true
	}
	return req.Wait || req.Sync || req.Blocking
}

func videoCandidateAccounts(req VideoGenerationRequest) ([]AccountRecord, error) {
	if strings.TrimSpace(req.AccountID) != "" {
		account, err := AppStore.GetAccount(strings.TrimSpace(req.AccountID))
		if err != nil {
			return nil, err
		}
		if !account.Enabled || !accountSupportsCapability(*account, "video") {
			return nil, sql.ErrNoRows
		}
		return []AccountRecord{*account}, nil
	}
	accounts, err := AppStore.ListRunnableAccountsForCapability("video")
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, sql.ErrNoRows
	}
	return accounts, nil
}

func normalizeTaskStatus(status string) string {
	switch status {
	case "processing", "running":
		return "running"
	case "succeeded", "completed":
		return "completed"
	case "cancelled", "canceled":
		return "cancelled"
	case "failed":
		return "failed"
	default:
		return "queued"
	}
}

func normalizeOpenAIVideoStatus(status string) string {
	switch status {
	case "running", "processing":
		return "in_progress"
	case "succeeded", "success":
		return "completed"
	case "cancelled", "canceled":
		return "cancelled"
	case "failed":
		return "failed"
	case "completed":
		return "completed"
	default:
		return "queued"
	}
}

func openAIVideoProgress(status string) int {
	switch status {
	case "completed", "failed", "cancelled":
		return 100
	case "in_progress":
		return 30
	default:
		return 0
	}
}

func taskRequestMap(task *TaskRecord) map[string]interface{} {
	if task == nil || strings.TrimSpace(task.RequestJSON) == "" {
		return nil
	}
	var req map[string]interface{}
	if json.Unmarshal([]byte(task.RequestJSON), &req) != nil {
		return nil
	}
	return req
}

func taskProviderModel(task *TaskRecord) string {
	req := taskRequestMap(task)
	if req == nil {
		return ""
	}
	for _, key := range []string{"provider_model", "upstream_model", "video_model"} {
		if value, ok := req[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	if task.Type == "video" {
		model := normalizeQianwenVideoModel(task.Model)
		return creatorModelSpec(model).UpstreamModel
	}
	if value, ok := req["model"].(string); ok {
		return value
	}
	return ""
}

func firstVideoURL(data interface{}) string {
	items, ok := data.([]interface{})
	if !ok || len(items) == 0 {
		return ""
	}
	first, ok := items[0].(map[string]interface{})
	if !ok {
		return ""
	}
	for _, key := range []string{"video_url", "url"} {
		if value, ok := first[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func freshOrOriginal(fresh, original *TaskRecord) *TaskRecord {
	if fresh != nil {
		return fresh
	}
	return original
}

func limitURLs(urls []string, n int) []string {
	if n <= 0 {
		n = 1
	}
	if len(urls) <= n {
		return urls
	}
	return urls[:n]
}

func parseUnix(value string) int64 {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Now().Unix()
	}
	return t.Unix()
}
