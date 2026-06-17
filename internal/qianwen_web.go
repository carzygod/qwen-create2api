package internal

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	qwenCreatorPageURL     = "https://create.qianwen.com/r/ai-studio-pc/main/gen-video"
	qwenCreatorOrigin      = "https://create.qianwen.com"
	qwenCreatorAPIBaseURL  = "https://ai-studio-create.qianwen.com"
	qwenCreatorResourceURL = "https://aistudio-resource.qianwen.com"
)

type qwenCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite"`
}

type qwenWebClient struct {
	account      AccountRecord
	httpClient   *http.Client
	cookieHeader string
	xsrfToken    string
	deviceID     string
	userAgent    string
	resourceAuth *creatorResourceAuth
}

type creatorResourceAuth struct {
	Dvidn  string
	Actkn  string
	Snver  string
	Bacsft []string
}

type creatorImageBlob struct {
	Bytes       []byte
	ContentType string
	FileName    string
	FileType    string
}

type qwenWebEvent struct {
	Raw map[string]interface{} `json:"raw,omitempty"`
}

type qwenRequestState struct {
	ReqID     string                 `json:"req_id"`
	SessionID string                 `json:"session_id"`
	DeviceID  string                 `json:"device_id"`
	RecordID  string                 `json:"record_id,omitempty"`
	Scene     string                 `json:"scene,omitempty"`
	Payload   map[string]interface{} `json:"payload"`
}

type mediaPollResult struct {
	URLs   []string       `json:"urls"`
	Events []qwenWebEvent `json:"events,omitempty"`
}

type creatorVideoModelSpec struct {
	PublicModel       string
	UpstreamModel     string
	RootModel         string
	Scene             string
	GenMode           string
	SupportsFirst     bool
	SupportsLast      bool
	TypedAttachments  bool
	AttachmentTypeKey bool
}

var httpURLPattern = regexp.MustCompile(`https?://[^\s"'<>\\)]+`)

func newQwenWebClient(account AccountRecord) (*qwenWebClient, error) {
	cookieHeader, xsrf := accountCookieHeader(account)
	if strings.TrimSpace(cookieHeader) == "" {
		return nil, fmt.Errorf("account has no Qianwen Creator cookie material")
	}
	userAgent := strings.TrimSpace(account.UserAgent)
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"
	}
	deviceID := strings.TrimSpace(account.DeviceID)
	if deviceID == "" {
		deviceID = uuid.New().String()
	}
	return &qwenWebClient{
		account:      account,
		httpClient:   &http.Client{Timeout: 180 * time.Second},
		cookieHeader: cookieHeader,
		xsrfToken:    xsrf,
		deviceID:     deviceID,
		userAgent:    userAgent,
	}, nil
}

func accountCookieHeader(account AccountRecord) (string, string) {
	var cookies []qwenCookie
	if strings.TrimSpace(account.CookieJSON) != "" {
		_ = json.Unmarshal([]byte(account.CookieJSON), &cookies)
	}
	if len(cookies) == 0 {
		return strings.TrimSpace(account.CookieString), cookieValueFromHeader(account.CookieString, "XSRF-TOKEN")
	}
	pairs := make([]string, 0, len(cookies))
	xsrf := ""
	for _, cookie := range cookies {
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}
		pairs = append(pairs, cookie.Name+"="+cookie.Value)
		if cookie.Name == "XSRF-TOKEN" {
			xsrf = cookie.Value
		}
	}
	return strings.Join(pairs, "; "), xsrf
}

func cookieValueFromHeader(header, name string) string {
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, name+"=") {
			return strings.TrimPrefix(part, name+"=")
		}
	}
	return ""
}

func (c *qwenWebClient) chat(ctx context.Context, req *ChatRequest) (string, []qwenWebEvent, error) {
	if err := c.probeSession(ctx); err != nil {
		return "", nil, err
	}
	return "ok", []qwenWebEvent{{Raw: map[string]interface{}{"probe": "creator-session"}}}, nil
}

func (c *qwenWebClient) probeSession(ctx context.Context) error {
	resp, err := c.postCreatorJSON(ctx, qwenCreatorAPIBaseURL, "/api/web/v1/user/identity", map[string]interface{}{})
	if err != nil {
		return err
	}
	if code := intFromAny(resp["code"]); code != 0 {
		return fmt.Errorf("Qianwen Creator session probe failed: code=%d msg=%s", code, stringFromAny(resp["msg"]))
	}
	return nil
}

func (c *qwenWebClient) submitImage(ctx context.Context, req ImageGenerationRequest) (*qwenRequestState, []qwenWebEvent, error) {
	return nil, nil, fmt.Errorf("QIANWEN-CREATOR-01 does not expose image generation in this first package; use video endpoints or add the /api/web/ai/image/function adapter")
}

func (c *qwenWebClient) submitVideo(ctx context.Context, req VideoGenerationRequest) (*qwenRequestState, []qwenWebEvent, error) {
	spec := creatorModelSpec(req.Model)
	if spec.UpstreamModel == "" {
		return nil, nil, fmt.Errorf("unsupported Qianwen Creator video model: %s", req.Model)
	}

	firstMaterial, err := c.resolveCreatorMaterialID(ctx, req.FirstFrameMaterialID, req.FirstFrameImage)
	if err != nil {
		return nil, nil, fmt.Errorf("first frame material failed: %w", err)
	}
	lastMaterial, err := c.resolveCreatorMaterialID(ctx, req.LastFrameMaterialID, req.LastFrameImage)
	if err != nil {
		return nil, nil, fmt.Errorf("last frame material failed: %w", err)
	}
	if spec.SupportsLast && firstMaterial == "" && lastMaterial != "" {
		return nil, nil, fmt.Errorf("Qianwen Creator does not support last-frame-only generation; first_frame_image or first_frame_material_id is required")
	}
	if !spec.SupportsLast && lastMaterial != "" {
		return nil, nil, fmt.Errorf("model %s does not expose last-frame control in the observed Creator Web protocol", req.Model)
	}
	if spec.SupportsFirst && firstMaterial == "" && spec.Scene != "wan25_txt_to_video" {
		return nil, nil, fmt.Errorf("model %s requires first_frame_image or first_frame_material_id", req.Model)
	}

	reqID := generateRandomHex(16)
	payload := c.buildCreatorVideoPayload(req, spec, firstMaterial, lastMaterial)
	resp, err := c.postCreatorJSONWithReqID(ctx, qwenCreatorAPIBaseURL, "/api/web/ai/video/function", payload, reqID)
	if err != nil {
		return nil, nil, err
	}
	if code := intFromAny(resp["code"]); code != 0 {
		return nil, []qwenWebEvent{{Raw: resp}}, fmt.Errorf("Qianwen Creator video submit failed: code=%d msg=%s", code, stringFromAny(resp["msg"]))
	}

	data := mapFromAny(resp["data"])
	recordID := firstNonEmptyString(
		stringFromAny(data["recordId"]),
		stringFromAny(data["record_id"]),
		stringFromAny(data["id"]),
		stringFromAny(data["taskId"]),
	)
	if recordID == "" {
		return nil, []qwenWebEvent{{Raw: resp}}, fmt.Errorf("Qianwen Creator video submit returned no recordId")
	}
	state := &qwenRequestState{
		ReqID:     reqID,
		SessionID: recordID,
		DeviceID:  c.deviceID,
		RecordID:  recordID,
		Scene:     spec.Scene,
		Payload:   payload,
	}
	return state, []qwenWebEvent{{Raw: resp}}, nil
}

func (c *qwenWebClient) buildCreatorVideoPayload(req VideoGenerationRequest, spec creatorVideoModelSpec, firstMaterial, lastMaterial string) map[string]interface{} {
	duration := req.Duration
	if duration <= 0 {
		duration = 5
	}
	resolution := strings.ToUpper(strings.TrimSpace(req.Resolution))
	if resolution == "" {
		resolution = "720P"
	}
	aspect := strings.TrimSpace(req.AspectRatio)
	if aspect == "" {
		aspect = "16:9"
	}

	attachments := []map[string]interface{}{}
	appendImage := func(materialID string) {
		if strings.TrimSpace(materialID) == "" {
			return
		}
		item := map[string]interface{}{"materialId": materialID}
		if spec.TypedAttachments {
			item["type"] = "image"
		}
		attachments = append(attachments, item)
	}
	appendImage(firstMaterial)
	appendImage(lastMaterial)
	for _, materialID := range req.ReferenceMaterialIDs {
		appendImage(materialID)
	}

	params := map[string]interface{}{
		"size":           aspect,
		"resolution":     resolution,
		"audio":          false,
		"attachments":    attachments,
		"duration":       duration,
		"attachmentType": 0,
	}

	payload := map[string]interface{}{
		"originPrompt": req.Prompt,
		"prompt":       req.Prompt,
		"scene":        spec.Scene,
		"model":        spec.UpstreamModel,
		"rootModel":    spec.RootModel,
		"params":       params,
	}
	if spec.GenMode != "" {
		payload["genMode"] = spec.GenMode
	}
	return payload
}

func (c *qwenWebClient) resolveCreatorMaterialID(ctx context.Context, explicitMaterialID, imageValue string) (string, error) {
	if strings.TrimSpace(explicitMaterialID) != "" {
		return strings.TrimSpace(explicitMaterialID), nil
	}
	imageValue = strings.TrimSpace(imageValue)
	if imageValue == "" {
		return "", nil
	}
	if looksLikeMaterialID(imageValue) {
		return imageValue, nil
	}
	lower := strings.ToLower(imageValue)
	if strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		blob, err := c.loadCreatorImageBlob(ctx, imageValue)
		if err != nil {
			return "", err
		}
		return c.uploadCreatorImageBlob(ctx, blob)
	}
	return "", fmt.Errorf("unsupported image value; pass a material id, public image URL, or data URI")
}

func (c *qwenWebClient) loadCreatorImageBlob(ctx context.Context, imageValue string) (*creatorImageBlob, error) {
	if strings.HasPrefix(strings.ToLower(imageValue), "data:") {
		return decodeCreatorDataURI(imageValue)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageValue, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download image failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("download image status %d: %s", resp.StatusCode, string(body))
	}
	body, err := readLimited(resp.Body, 20*1024*1024)
	if err != nil {
		return nil, err
	}
	contentType := normalizeImageContentType(resp.Header.Get("Content-Type"), body)
	if !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("downloaded URL is not an image: %s", contentType)
	}
	fileName := creatorFileNameFromURL(imageValue, contentType)
	return &creatorImageBlob{
		Bytes:       body,
		ContentType: contentType,
		FileName:    fileName,
		FileType:    contentType,
	}, nil
}

func decodeCreatorDataURI(value string) (*creatorImageBlob, error) {
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid data URI")
	}
	meta := strings.TrimPrefix(parts[0], "data:")
	isBase64 := strings.Contains(strings.ToLower(meta), ";base64")
	contentType := strings.Split(meta, ";")[0]
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	var raw []byte
	var err error
	if isBase64 {
		raw, err = base64.StdEncoding.DecodeString(parts[1])
	} else {
		var decoded string
		decoded, err = url.PathUnescape(parts[1])
		raw = []byte(decoded)
	}
	if err != nil {
		return nil, fmt.Errorf("decode data URI failed: %w", err)
	}
	contentType = normalizeImageContentType(contentType, raw)
	if !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("data URI is not an image: %s", contentType)
	}
	return &creatorImageBlob{
		Bytes:       raw,
		ContentType: contentType,
		FileName:    "upload" + creatorExtFromContentType(contentType),
		FileType:    contentType,
	}, nil
}

func (c *qwenWebClient) uploadCreatorImageBlob(ctx context.Context, blob *creatorImageBlob) (string, error) {
	if blob == nil || len(blob.Bytes) == 0 {
		return "", fmt.Errorf("empty image payload")
	}
	sum := md5.Sum(blob.Bytes)
	md5Base64 := base64.StdEncoding.EncodeToString(sum[:])
	fileType := blob.FileType
	if fileType == "" {
		fileType = blob.ContentType
	}
	tokenPayload := map[string]interface{}{
		"file_name":    blob.FileName,
		"content_type": "application/octet-stream",
		"content_md5":  md5Base64,
		"size":         strconv.Itoa(len(blob.Bytes)),
		"file_type":    fileType,
		"entry":        "ugc",
	}
	tokenResp, err := c.postCreatorResourceJSON(ctx, "/1/oss_token", tokenPayload, generateChid())
	if err != nil {
		return "", err
	}
	if code := intFromAny(tokenResp["code"]); code != 0 {
		return "", fmt.Errorf("Creator OSS token failed: code=%d msg=%s", code, stringFromAny(tokenResp["msg"]))
	}
	tokenData := mapFromAny(tokenResp["data"])
	if err := c.putCreatorOSSObject(ctx, tokenData, blob, md5Base64); err != nil {
		return "", err
	}
	objectName := stringFromAny(tokenData["object"])
	bucket := stringFromAny(tokenData["bucket"])
	endpoint := stringFromAny(tokenData["endpoint"])
	if objectName == "" || bucket == "" || endpoint == "" {
		return "", fmt.Errorf("Creator OSS token response missing object/bucket/endpoint")
	}
	payload := map[string]interface{}{
		"object":    objectName,
		"bucket":    bucket,
		"file_name": blob.FileName,
		"file_md5":  md5Base64,
		"file_type": creatorCallbackFileType(blob.FileName, blob.ContentType),
		"entry":     "ugc",
		"endpoint":  endpoint,
	}
	callbackResp, err := c.postCreatorResourceJSON(ctx, "/1/oss/callback", payload, generateChid())
	if err != nil {
		return "", err
	}
	if code := intFromAny(callbackResp["code"]); code != 0 {
		return "", fmt.Errorf("Creator OSS callback failed: code=%d msg=%s", code, stringFromAny(callbackResp["msg"]))
	}
	data := mapFromAny(callbackResp["data"])
	materialID := firstNonEmptyString(
		stringFromAny(data["material_id"]),
		stringFromAny(data["materialId"]),
		stringFromAny(data["id"]),
	)
	if materialID == "" {
		return "", fmt.Errorf("Creator OSS callback returned no material_id")
	}
	return materialID, nil
}

func (c *qwenWebClient) putCreatorOSSObject(ctx context.Context, tokenData map[string]interface{}, blob *creatorImageBlob, md5Base64 string) error {
	host := stringFromAny(tokenData["host"])
	objectName := stringFromAny(tokenData["object"])
	authorization := stringFromAny(tokenData["authorization"])
	if host == "" || objectName == "" || authorization == "" {
		return fmt.Errorf("Creator OSS token response missing host/object/authorization")
	}
	reqID := generateChid()
	putURL := fmt.Sprintf("%s/%s?%s", strings.TrimRight(host, "/"), url.PathEscape(objectName), creatorResourceQuery(reqID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, bytes.NewReader(blob.Bytes))
	if err != nil {
		return err
	}
	contentType := firstNonEmptyString(stringFromAny(tokenData["content_type"]), "application/octet-stream")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", qwenCreatorOrigin)
	req.Header.Set("Referer", qwenCreatorOrigin+"/")
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Content-MD5", md5Base64)
	req.Header.Set("Authorization", authorization)
	for _, h := range sliceFromAny(tokenData["oss_headers"]) {
		item := mapFromAny(h)
		key := stringFromAny(item["key"])
		value := stringFromAny(item["value"])
		if key != "" {
			req.Header.Set(key, value)
		}
	}
	if err := c.setCreatorResourceSecurityHeaders(ctx, req, reqID); err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Creator OSS put failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Creator OSS put status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *qwenWebClient) postCreatorResourceJSON(ctx context.Context, path string, payload map[string]interface{}, reqID string) (map[string]interface{}, error) {
	body, _ := json.Marshal(payload)
	reqURL := fmt.Sprintf("%s%s?%s", strings.TrimRight(qwenCreatorResourceURL, "/"), path, creatorResourceQuery(reqID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", qwenCreatorOrigin)
	req.Header.Set("Referer", qwenCreatorOrigin+"/")
	req.Header.Set("Cookie", c.cookieHeader)
	if err := c.setCreatorResourceSecurityHeaders(ctx, req, reqID); err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Qianwen Creator Resource status %d: %s", resp.StatusCode, string(raw))
	}
	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Qianwen Creator Resource response: %w; body=%s", err, string(raw))
	}
	return result, nil
}

func creatorResourceQuery(reqID string) string {
	values := url.Values{}
	values.Set("biz_id", "ai_image")
	values.Set("req_id", reqID)
	values.Set("uc_param_str", "vesvutkpfrcgprospc")
	values.Set("pr", "kkpcweb")
	values.Set("fr", "win")
	return values.Encode()
}

func (c *qwenWebClient) setCreatorResourceSecurityHeaders(ctx context.Context, req *http.Request, reqID string) error {
	auth, err := c.ensureCreatorResourceAuth(ctx)
	if err != nil {
		return err
	}
	sacsft, ok := auth.consumeBacsft()
	if !ok {
		c.resourceAuth = nil
		auth, err = c.ensureCreatorResourceAuth(ctx)
		if err != nil {
			return err
		}
		sacsft, ok = auth.consumeBacsft()
		if !ok {
			return fmt.Errorf("Creator Resource signer has no bacsft token")
		}
	}
	reqt := time.Now().UnixMilli()
	ve := "1.0.0"
	kp := ""
	signature := GenerateSignature(auth.Dvidn, ve, kp, reqID, sacsft, reqt)
	req.Header.Set("clt-acs-sign", signature)
	req.Header.Set("clt-acs-reqt", fmt.Sprintf("%d", reqt))
	req.Header.Set("clt-acs-request-params", "req_id")
	req.Header.Set("clt-acs-caer", "vrad")
	req.Header.Set("eo-clt-dvidn", auth.Dvidn)
	req.Header.Set("eo-clt-sacsft", sacsft)
	req.Header.Set("eo-clt-snver", auth.Snver)
	req.Header.Set("eo-clt-actkn", auth.Actkn)
	req.Header.Set("eo-clt-acs-ve", ve)
	req.Header.Set("eo-clt-acs-kp", kp)
	return nil
}

func (a *creatorResourceAuth) consumeBacsft() (string, bool) {
	if a == nil || len(a.Bacsft) == 0 {
		return "", false
	}
	token := a.Bacsft[0]
	a.Bacsft = a.Bacsft[1:]
	return token, true
}

func (c *qwenWebClient) ensureCreatorResourceAuth(ctx context.Context) (*creatorResourceAuth, error) {
	if c.resourceAuth != nil && c.resourceAuth.Dvidn != "" && c.resourceAuth.Actkn != "" && len(c.resourceAuth.Bacsft) > 0 {
		return c.resourceAuth, nil
	}
	umid, err := GenerateUMIDTokenWithRetry(3)
	if err != nil {
		return nil, fmt.Errorf("generate Creator Resource UMID token failed: %w", err)
	}
	payload := map[string]interface{}{
		"screenResolution":    "1440x950",
		"cookieEnabled":       true,
		"localStorageEnabled": true,
		"timezoneOffset":      "Asia/Shanghai",
		"fontList":            []string{"Arial", "Calibri", "Century", "Century Gothic", "SimHei", "Segoe UI Light"},
		"pluginList":          []interface{}{},
		"language":            []string{"zh-CN"},
		"unifyRelateGenerate": []string{},
		"fingerprint":         generateFingerprint(c.deviceID),
		"businessScene":       "quark_web",
		"chid":                generateChid(),
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://ext.quark.cn/security/external/access/register", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", qwenCreatorOrigin)
	req.Header.Set("Referer", qwenCreatorOrigin+"/")
	req.Header.Set("bx-umidtoken", umid)
	req.Header.Set("clt-acs-caer", "vrad")
	req.Header.Set("eo-clt-sftcnt", "20")
	if c.cookieHeader != "" {
		req.Header.Set("Cookie", c.cookieHeader)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Creator Resource access register failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Creator Resource access register status %d: %s", resp.StatusCode, string(raw))
	}
	var result RegisterResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse Creator Resource access register response failed: %w; body=%s", err, string(raw))
	}
	if result.Status != 0 {
		return nil, fmt.Errorf("Creator Resource access register failed: code=%s msg=%s", result.Code, result.Msg)
	}
	auth := creatorResourceAuth{
		Dvidn:  result.Data.EoCltDvidn,
		Snver:  result.Data.EoCltSnver,
		Bacsft: append([]string{}, result.Data.EoCltBacsft...),
	}
	for _, relate := range result.Data.UnifyRelate {
		if relate.BusinessScene == "workspace_api" {
			auth.Actkn = relate.EoCltActkn
			auth.Bacsft = append([]string{}, relate.EoCltBacsft...)
			if relate.EoCltSnver != "" {
				auth.Snver = relate.EoCltSnver
			}
			break
		}
	}
	if auth.Actkn == "" {
		for _, relate := range result.Data.UnifyRelate {
			if relate.EoCltActkn != "" && len(relate.EoCltBacsft) > 0 {
				auth.Actkn = relate.EoCltActkn
				auth.Bacsft = append([]string{}, relate.EoCltBacsft...)
				if relate.EoCltSnver != "" {
					auth.Snver = relate.EoCltSnver
				}
				break
			}
		}
	}
	if auth.Dvidn == "" || auth.Actkn == "" || auth.Snver == "" || len(auth.Bacsft) == 0 {
		return nil, fmt.Errorf("Creator Resource access register returned incomplete signer material")
	}
	c.resourceAuth = &auth
	return c.resourceAuth, nil
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	lr := io.LimitReader(r, limit+1)
	body, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("image is larger than %d bytes", limit)
	}
	return body, nil
}

func normalizeImageContentType(contentType string, body []byte) string {
	contentType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(body)
	}
	switch contentType {
	case "image/jpg":
		return "image/jpeg"
	default:
		return contentType
	}
}

func creatorFileNameFromURL(rawURL, contentType string) string {
	parsed, err := url.Parse(rawURL)
	name := ""
	if err == nil {
		name = filepath.Base(parsed.Path)
	}
	if name == "." || name == "/" || name == "" {
		name = fmt.Sprintf("upload_%d%s", time.Now().UnixMilli(), creatorExtFromContentType(contentType))
	}
	if filepath.Ext(name) == "" {
		name += creatorExtFromContentType(contentType)
	}
	return sanitizeCreatorFileName(name)
}

func creatorExtFromContentType(contentType string) string {
	switch strings.ToLower(strings.Split(contentType, ";")[0]) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		if exts, err := mime.ExtensionsByType(contentType); err == nil && len(exts) > 0 {
			return exts[0]
		}
		return ".jpg"
	}
}

func sanitizeCreatorFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "" {
		return fmt.Sprintf("upload_%d.jpg", time.Now().UnixMilli())
	}
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return replacer.Replace(name)
}

func creatorCallbackFileType(fileName, contentType string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")
	if ext == "" {
		ext = strings.TrimPrefix(creatorExtFromContentType(contentType), ".")
	}
	if ext == "jpeg" {
		ext = "jpg"
	}
	return strings.ToUpper(ext)
}

func sliceFromAny(value interface{}) []interface{} {
	if items, ok := value.([]interface{}); ok {
		return items
	}
	return nil
}

func (c *qwenWebClient) pollSnap(ctx context.Context, state *qwenRequestState) ([]qwenWebEvent, error) {
	if state == nil || strings.TrimSpace(state.RecordID) == "" {
		return nil, fmt.Errorf("missing Qianwen Creator record id")
	}
	payload := map[string]interface{}{
		"recordId": state.RecordID,
		"scene":    state.Scene,
	}
	resp, err := c.postCreatorJSONWithReqID(ctx, qwenCreatorAPIBaseURL, "/api/web/ai/video/record/query", payload, state.ReqID)
	if err != nil {
		return nil, err
	}
	if code := intFromAny(resp["code"]); code != 0 {
		return []qwenWebEvent{{Raw: resp}}, fmt.Errorf("Qianwen Creator record query failed: code=%d msg=%s", code, stringFromAny(resp["msg"]))
	}
	return []qwenWebEvent{{Raw: resp}}, nil
}

func (c *qwenWebClient) pollMedia(ctx context.Context, state *qwenRequestState, mediaType string, timeout time.Duration) (*mediaPollResult, error) {
	deadline := time.Now().Add(timeout)
	var lastEvents []qwenWebEvent
	for {
		events, err := c.pollSnap(ctx, state)
		if err != nil {
			return nil, err
		}
		lastEvents = events
		urls := filterMediaURLs(extractURLs(events), mediaType)
		if len(urls) > 0 {
			return &mediaPollResult{URLs: urls, Events: events}, nil
		}
		status := creatorStatus(events)
		if status == "failed" || status == "auditFailed" || status == "audit_failed" {
			return &mediaPollResult{Events: events}, fmt.Errorf("Qianwen Creator task failed: %s", marshalCompact(events))
		}
		if time.Now().After(deadline) {
			return &mediaPollResult{Events: lastEvents}, fmt.Errorf("Qianwen Creator %s generation did not return media url before timeout", mediaType)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func (c *qwenWebClient) postCreatorJSON(ctx context.Context, baseURL, path string, payload map[string]interface{}) (map[string]interface{}, error) {
	return c.postCreatorJSONWithReqID(ctx, baseURL, path, payload, generateRandomHex(16))
}

func (c *qwenWebClient) postCreatorJSONWithReqID(ctx context.Context, baseURL, path string, payload map[string]interface{}, reqID string) (map[string]interface{}, error) {
	bodyPayload, chid, ts := c.withCreatorRuntimeFields(baseURL, payload)
	body, _ := json.Marshal(bodyPayload)
	url := creatorAPIURL(baseURL, path, reqID, chid, ts)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setCreatorHeaders(req, baseURL, reqID)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Qianwen Creator upstream status %d: %s", resp.StatusCode, string(raw))
	}
	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Qianwen Creator response: %w; body=%s", err, string(raw))
	}
	return result, nil
}

func (c *qwenWebClient) withCreatorRuntimeFields(baseURL string, payload map[string]interface{}) (map[string]interface{}, string, int64) {
	chid := generateChid()
	ts := time.Now().UnixMilli()
	if !strings.Contains(baseURL, "ai-studio") {
		return payload, chid, ts
	}
	next := make(map[string]interface{}, len(payload)+5)
	for key, value := range payload {
		next[key] = value
	}
	if _, ok := next["chid"]; !ok {
		next["chid"] = chid
	}
	if _, ok := next["product"]; !ok {
		next["product"] = "ai_studio"
	}
	if _, ok := next["browserId"]; !ok {
		next["browserId"] = c.deviceID
	}
	if _, ok := next["timestamp"]; !ok {
		next["timestamp"] = ts
	}
	if _, ok := next["platform"]; !ok {
		next["platform"] = "pc"
	}
	return next, chid, ts
}

func creatorAPIURL(baseURL, path, reqID, chid string, ts int64) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%s%sbiz_id=ai_image&req_id=%s&ai_ts=%d&chid=%s&platform=pc&pr=qianwen&fr=pc", strings.TrimRight(baseURL, "/"), path, sep, reqID, ts, chid)
}

func (c *qwenWebClient) setCreatorHeaders(req *http.Request, baseURL, reqID string) {
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", qwenCreatorOrigin)
	req.Header.Set("Referer", qwenCreatorPageURL)
	req.Header.Set("Cookie", c.cookieHeader)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("x-requested-with", "XMLHttpRequest")
	req.Header.Set("x-device-id", c.deviceID)
	req.Header.Set("x-wpk-reqid", reqID)
	req.Header.Set("x-wpk-traceid", reqID)
	if c.xsrfToken != "" {
		req.Header.Set("x-csrf-token", c.xsrfToken)
	}
	if strings.Contains(baseURL, "aistudio-resource") {
		req.Header.Set("Origin", qwenCreatorOrigin)
	}
}

func creatorModelSpec(model string) creatorVideoModelSpec {
	switch normalizeQianwenVideoModel(model) {
	case "qianwen-creator-wan21-frame":
		return creatorVideoModelSpec{
			PublicModel:      "qianwen-creator-wan21-frame",
			UpstreamModel:    "wanx2.1-kf2v-plus",
			RootModel:        "wan21",
			Scene:            "frame_image_to_video",
			GenMode:          "vid_gen",
			SupportsFirst:    true,
			SupportsLast:     true,
			TypedAttachments: true,
		}
	case "qianwen-creator-wan22-flash-frame":
		return creatorVideoModelSpec{
			PublicModel:      "qianwen-creator-wan22-flash-frame",
			UpstreamModel:    "wan2.2-kf2v-flash",
			RootModel:        "wan22_flash",
			Scene:            "wan22_flash_frame_itv",
			GenMode:          "vid_gen",
			SupportsFirst:    true,
			SupportsLast:     true,
			TypedAttachments: true,
		}
	case "qianwen-creator-wan25-i2v":
		return creatorVideoModelSpec{
			PublicModel:      "qianwen-creator-wan25-i2v",
			UpstreamModel:    "wan2.5-i2v-preview",
			RootModel:        "wan25",
			Scene:            "wan25_first_frame_itv",
			GenMode:          "vid_gen",
			SupportsFirst:    true,
			TypedAttachments: true,
		}
	case "qianwen-creator-wan25-t2v":
		return creatorVideoModelSpec{
			PublicModel:   "qianwen-creator-wan25-t2v",
			UpstreamModel: "wan2.5-t2v-preview",
			RootModel:     "wan25",
			Scene:         "wan25_txt_to_video",
			GenMode:       "vid_gen",
		}
	case "qianwen-creator-wan27-frame":
		return creatorVideoModelSpec{
			PublicModel:      "qianwen-creator-wan27-frame",
			UpstreamModel:    "wan2.7-i2v",
			RootModel:        "wan27",
			Scene:            "wan27_frame_i2v",
			GenMode:          "vid_gen",
			SupportsFirst:    true,
			SupportsLast:     true,
			TypedAttachments: true,
		}
	case "qianwen-creator-happyhorse-i2v":
		return creatorVideoModelSpec{
			PublicModel:      "qianwen-creator-happyhorse-i2v",
			UpstreamModel:    "happyhorse",
			RootModel:        "happyhorse",
			Scene:            "hh_first_frame_i2v",
			GenMode:          "vid_gen",
			SupportsFirst:    true,
			TypedAttachments: true,
		}
	default:
		return creatorVideoModelSpec{}
	}
}

func creatorStatus(events []qwenWebEvent) string {
	for _, event := range events {
		status := findStringDeep(event.Raw, "status", "taskStatus", "task_status", "state")
		if status != "" {
			return status
		}
	}
	return ""
}

func findStringDeep(value interface{}, keys ...string) string {
	keySet := map[string]bool{}
	for _, key := range keys {
		keySet[strings.ToLower(key)] = true
	}
	var walk func(interface{}) string
	walk = func(v interface{}) string {
		switch typed := v.(type) {
		case map[string]interface{}:
			for k, item := range typed {
				if keySet[strings.ToLower(k)] {
					if s := stringFromAny(item); s != "" {
						return s
					}
				}
			}
			for _, item := range typed {
				if s := walk(item); s != "" {
					return s
				}
			}
		case []interface{}:
			for _, item := range typed {
				if s := walk(item); s != "" {
					return s
				}
			}
		}
		return ""
	}
	return walk(value)
}

func extractURLs(value interface{}) []string {
	raw, _ := json.Marshal(value)
	matches := httpURLPattern.FindAllString(string(raw), -1)
	out := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, match := range matches {
		match = html.UnescapeString(strings.TrimRight(match, ".,;"))
		match = strings.ReplaceAll(match, `\/`, `/`)
		if seen[match] {
			continue
		}
		seen[match] = true
		out = append(out, match)
	}
	return out
}

func filterMediaURLs(urls []string, mediaType string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, u := range urls {
		lower := strings.ToLower(u)
		if strings.Contains(lower, "g.alicdn.com") || strings.Contains(lower, "w3.org") {
			continue
		}
		if mediaType == "video" {
			if !strings.Contains(lower, ".mp4") && !strings.Contains(lower, "video") {
				continue
			}
		} else {
			if strings.Contains(lower, ".mp4") || !(strings.Contains(lower, ".png") || strings.Contains(lower, ".jpg") || strings.Contains(lower, ".jpeg") || strings.Contains(lower, ".webp")) {
				continue
			}
		}
		if !(strings.Contains(lower, "workspace") || strings.Contains(lower, "quark") || strings.Contains(lower, "ai-studio") || strings.Contains(lower, "aistudio")) {
			continue
		}
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

func chatPrompt(messages []Message) string {
	if len(messages) == 0 {
		return "Hello"
	}
	if len(messages) == 1 {
		return messages[0].Content
	}
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		parts = append(parts, strings.ToUpper(role)+": "+msg.Content)
	}
	return strings.Join(parts, "\n")
}

func normalizeImageAspect(req ImageGenerationRequest) string {
	value := strings.TrimSpace(req.AspectRatio)
	if value == "" {
		value = strings.TrimSpace(req.Size)
	}
	if strings.Contains(value, ":") {
		return value
	}
	switch value {
	case "1024x1024", "1x1":
		return "1:1"
	case "1024x1792", "9x16":
		return "9:16"
	case "1792x1024", "16x9":
		return "16:9"
	default:
		return "3:4"
	}
}

func mustJSONString(value interface{}) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func marshalCompact(value interface{}) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func mapFromAny(value interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	if m, ok := value.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

func stringFromAny(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case float64:
		return fmt.Sprintf("%.0f", typed)
	case int:
		return fmt.Sprintf("%d", typed)
	default:
		return ""
	}
}

func intFromAny(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	case json.Number:
		i, _ := typed.Int64()
		return int(i)
	default:
		return 0
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
