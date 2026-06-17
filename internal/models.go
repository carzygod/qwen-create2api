package internal

import (
	"encoding/json"
	"net/http"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type QianwenContent struct {
	Content     string `json:"content"`
	ContentType string `json:"contentType"`
	Role        string `json:"role"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type ChatCompletionChunk struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Index        int          `json:"index"`
	Delta        Delta        `json:"delta,omitempty"`
	Message      *MessageResp `json:"message,omitempty"`
	FinishReason *string      `json:"finish_reason"`
}

type Delta struct {
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type MessageResp struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

type ModelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

type QianwenResponse struct {
	MsgStatus   string `json:"msgStatus"`
	Incremental bool   `json:"incremental"`
	Contents    []struct {
		Content     string `json:"content"`
		ContentType string `json:"contentType"`
		Status      string `json:"status"`
		CardCode    string `json:"cardCode"`
		Incremental bool   `json:"incremental"`
	} `json:"contents"`
	StopReason string `json:"stopReason"`
	ErrorCode  string `json:"errorCode"`
	ErrorMsg   string `json:"errorMsg"`
}

type GuestTicketResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Ticket string `json:"ticket"`
	} `json:"data"`
}

type RegisterResponse struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	Msg    string `json:"msg"`
	Data   struct {
		UnifyRelate []struct {
			BusinessScene string   `json:"businessScene"`
			EoCltActkn    string   `json:"eo-clt-actkn"`
			EoCltActknDl  int64    `json:"eo-clt-actkn-dl"`
			EoCltBacsft   []string `json:"eo-clt-bacsft"`
		} `json:"unifyRelate"`
		EoCltDvidn  string   `json:"eo-clt-dvidn"`
		EoCltSnver  string   `json:"eo-clt-snver"`
		EoCltBacsft []string `json:"eo-clt-bacsft"`
	} `json:"data"`
}

var ModelList = []string{
	"qianwen-creator-session",
	"qianwen-creator-wan21-frame",
	"qianwen-creator-wan22-flash-frame",
	"qianwen-creator-wan25-i2v",
	"qianwen-creator-wan25-t2v",
	"qianwen-creator-wan27-frame",
	"qianwen-creator-happyhorse-i2v",
}

func HandleModels(w http.ResponseWriter, r *http.Request) {
	var models []ModelInfo
	if AppStore != nil {
		rows, err := AppStore.ListModels()
		if err == nil {
			for _, model := range rows {
				models = append(models, ModelInfo{
					ID:      model.ID,
					Object:  "model",
					OwnedBy: "qianwen-creator",
				})
			}
		} else {
			LogWarn("Failed to list models from sqlite: %v", err)
		}
	}
	if len(models) == 0 {
		for _, id := range ModelList {
			models = append(models, ModelInfo{
				ID:      id,
				Object:  "model",
				OwnedBy: "qianwen-creator",
			})
		}
	}

	response := ModelsResponse{
		Object: "list",
		Data:   models,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
