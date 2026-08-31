package handler

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/quiztrace/quiztrace/internal/ai/embedding"
	"github.com/quiztrace/quiztrace/internal/ai/provider"
	"github.com/quiztrace/quiztrace/internal/ai/rerank"
	"github.com/quiztrace/quiztrace/internal/api"
	"github.com/quiztrace/quiztrace/internal/security"
)

// ProviderHandler AI Provider 管理（支持 LLM、Embedding、Rerank 分开独立管理）。
type ProviderHandler struct {
	registry *provider.Registry
}

func NewProviderHandler(registry *provider.Registry) *ProviderHandler {
	return &ProviderHandler{registry: registry}
}

func (h *ProviderHandler) List(c *gin.Context) {
	providerType := c.Query("type")
	list, err := h.registry.List(c.Request.Context(), providerType)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "查询 Provider 列表失败")
		return
	}
	api.OK(c, list)
}

type saveProviderRequest struct {
	Type           string `json:"provider_type"` // llm | embedding | rerank
	Name           string `json:"name" binding:"required"`
	Protocol       string `json:"protocol"`
	BaseURL        string `json:"base_url" binding:"required"`
	APIKey         string `json:"api_key"`
	ModelName      string `json:"model_name" binding:"required"`
	IsDefault      bool   `json:"is_default"`
	SupportsVision bool   `json:"supports_vision"`
}

func (h *ProviderHandler) Save(c *gin.Context) {
	var req saveProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法: "+err.Error())
		return
	}
	if req.Type == "" {
		req.Type = provider.TypeLLM
	}
	if req.Protocol == "" {
		req.Protocol = "openai"
	}
	if req.APIKey == "" {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "API Key 不能为空")
		return
	}
	if req.Type == provider.TypeLLM && req.SupportsVision {
		if _, err := probeVision(c.Request.Context(), req); err != nil {
			api.FailDetail(c, http.StatusBadGateway, api.ErrModelError, "视觉能力验证失败，配置未保存", err.Error())
			return
		}
	}
	p, err := h.registry.Save(c.Request.Context(), req.Type, req.Name, req.Protocol, req.BaseURL, req.APIKey, req.ModelName, req.IsDefault, req.SupportsVision)
	if err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "保存 Provider 失败: "+err.Error())
		return
	}
	api.Created(c, p)
}

func (h *ProviderHandler) Update(c *gin.Context) {
	id := parseID(c.Param("id"))
	var req saveProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	if req.SupportsVision {
		current, err := h.registry.Get(c.Request.Context(), id)
		if err != nil {
			api.Fail(c, http.StatusNotFound, api.ErrNotFound, "Provider 不存在")
			return
		}
		if req.Protocol == "" {
			req.Protocol = current.Protocol
		}
		var probeErr error
		if req.APIKey == "" {
			var llm provider.LLMProvider
			llm, probeErr = h.registry.NewLLM(c.Request.Context(), id)
			if probeErr == nil {
				_, probeErr = probeVisionLLM(c.Request.Context(), llm)
			}
		} else {
			_, probeErr = probeVision(c.Request.Context(), req)
		}
		if probeErr != nil {
			api.FailDetail(c, http.StatusBadGateway, api.ErrModelError, "视觉能力验证失败，配置未更新", probeErr.Error())
			return
		}
	}
	if err := h.registry.Update(c.Request.Context(), id, req.Name, req.BaseURL, req.APIKey, req.ModelName, req.IsDefault, req.SupportsVision); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "更新 Provider 失败: "+err.Error())
		return
	}
	api.OK(c, gin.H{"updated": true})
}

func (h *ProviderHandler) SetDefault(c *gin.Context) {
	id := parseID(c.Param("id"))
	if err := h.registry.SetDefault(c.Request.Context(), id); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "设置默认 Provider 失败: "+err.Error())
		return
	}
	api.OK(c, gin.H{"default": true})
}

func (h *ProviderHandler) Delete(c *gin.Context) {
	id := parseID(c.Param("id"))
	if err := h.registry.Delete(c.Request.Context(), id); err != nil {
		api.Fail(c, http.StatusInternalServerError, api.ErrInternal, "删除失败: "+err.Error())
		return
	}
	api.OK(c, gin.H{"deleted": true})
}

// TestStored 测试已经保存的 Provider。
// API Key 由服务端按当前用户从数据库解密，客户端不需要、也不应该再次提交密钥。
func (h *ProviderHandler) TestStored(c *gin.Context) {
	id := parseID(c.Param("id"))
	if id <= 0 {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "Provider ID 不合法")
		return
	}
	resolved, err := h.registry.Resolve(c.Request.Context(), id)
	if err != nil {
		api.Fail(c, http.StatusNotFound, api.ErrNotFound, "Provider 不存在或无权访问")
		return
	}
	h.testConfig(c, saveProviderRequest{
		Type:           resolved.Type,
		Name:           resolved.Name,
		Protocol:       resolved.Protocol,
		BaseURL:        resolved.BaseURL,
		APIKey:         resolved.APIKey,
		ModelName:      resolved.ModelName,
		SupportsVision: resolved.SupportsVision,
	})
}

// Test 测试未保存的模型配置（按 provider_type 分别测试 LLM, Embedding, Rerank）。
func (h *ProviderHandler) Test(c *gin.Context) {
	var req saveProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "参数不合法")
		return
	}
	h.testConfig(c, req)
}

func (h *ProviderHandler) testConfig(c *gin.Context, req saveProviderRequest) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	start := time.Now()

	switch req.Type {
	case provider.TypeEmbedding:
		emb, err := embedding.NewProvider(embedding.Config{
			Protocol: req.Protocol,
			BaseURL:  req.BaseURL,
			APIKey:   req.APIKey,
			Model:    req.ModelName,
		})
		if err != nil {
			api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "Embedding 配置不合法: "+err.Error())
			return
		}
		vectors, err := emb.Embed(ctx, []string{"题迹 QuizTrace 连通性测试文本"})
		latency := time.Since(start).Milliseconds()
		if err != nil {
			api.FailDetail(c, http.StatusBadGateway, api.ErrModelError, "Embedding 连接失败", err.Error())
			return
		}
		dim := 0
		if len(vectors) > 0 {
			dim = len(vectors[0])
		}
		api.OK(c, gin.H{
			"ok":         true,
			"type":       provider.TypeEmbedding,
			"model":      req.ModelName,
			"dimension":  dim,
			"latency_ms": latency,
			"reply":      fmt.Sprintf("成功生成向量（维度: %d）", dim),
		})

	case provider.TypeRerank:
		reranker := rerank.NewProvider(rerank.Config{
			Protocol:   req.Protocol,
			BaseURL:    req.BaseURL,
			APIKey:     req.APIKey,
			Model:      req.ModelName,
			TimeoutSec: 15,
		})
		testItems := []rerank.Item{
			{ID: 1, Content: "操作系统中的死锁概念与必要条件", Score: 0.5},
			{ID: 2, Content: "HTTP/2 多路复用与头部压缩原理", Score: 0.3},
		}
		ranked, err := reranker.Rerank(ctx, "计算机网络协议", testItems, 2)
		latency := time.Since(start).Milliseconds()
		if err != nil {
			api.FailDetail(c, http.StatusBadGateway, api.ErrModelError, "Rerank 连接失败: "+err.Error(), err.Error())
			return
		}
		api.OK(c, gin.H{
			"ok":         true,
			"type":       provider.TypeRerank,
			"model":      req.ModelName,
			"latency_ms": latency,
			"reply":      fmt.Sprintf("重排测试成功（返回 %d 条排序结果）", len(ranked)),
		})

	case provider.TypeLLM, "":
		cfg := provider.Config{
			Name: req.Name, Protocol: req.Protocol, BaseURL: req.BaseURL, APIKey: req.APIKey,
			ChatModel: req.ModelName, TimeoutSec: 30, SupportsVision: req.SupportsVision,
		}
		llm, err := provider.NewFromConfig(cfg)
		if err != nil {
			api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "LLM 配置不合法: "+err.Error())
			return
		}
		message := provider.ChatMessage{Role: "user", Content: "回复 OK"}
		if req.SupportsVision {
			message = visionProbeMessage()
		}
		probeTokens := 40
		if req.SupportsVision {
			// Vision preflight only checks whether the provider accepts an image.
			// Do not require a tiny JSON response: thinking-enabled models can
			// spend the budget on reasoning and return a truncated JSON payload.
			probeTokens = 128
		}
		resp, err := llm.Chat(ctx, provider.ChatRequest{Messages: []provider.ChatMessage{message}, MaxTokens: probeTokens, DisableThinking: req.SupportsVision})
		latency := time.Since(start).Milliseconds()
		if err != nil {
			api.FailDetail(c, http.StatusBadGateway, api.ErrModelError, "LLM 连接失败", err.Error())
			return
		}
		api.OK(c, gin.H{
			"ok":              true,
			"type":            provider.TypeLLM,
			"model":           resp.Model,
			"latency_ms":      latency,
			"reply":           security.Truncate(resp.Content, 50),
			"supports_vision": req.SupportsVision,
		})

	default:
		api.Fail(c, http.StatusBadRequest, api.ErrInvalidRequest, "不支持的 Provider 类型: "+req.Type)
	}
}

func visionProbeMessage() provider.ChatMessage {
	// Encode the probe image with the standard library instead of keeping a
	// hand-written byte slice. Some vision gateways validate the PNG chunks and
	// reject otherwise plausible but malformed tiny images.
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	var encoded bytes.Buffer
	_ = png.Encode(&encoded, img)
	return provider.ChatMessage{Role: "user", Content: "请读取所附图片，并仅回复 OK。", Parts: []provider.ContentPart{{Type: "image", MIMEType: "image/png", Data: encoded.Bytes(), Detail: "low"}}}
}

func probeVision(parent context.Context, req saveProviderRequest) (provider.ChatResponse, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	llm, err := provider.NewFromConfig(provider.Config{Name: req.Name, Protocol: req.Protocol, BaseURL: req.BaseURL, APIKey: req.APIKey, ChatModel: req.ModelName, TimeoutSec: 30, SupportsVision: true})
	if err != nil {
		return provider.ChatResponse{}, err
	}
	return probeVisionLLM(ctx, llm)
}

func probeVisionLLM(parent context.Context, llm provider.LLMProvider) (provider.ChatResponse, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	resp, err := llm.Chat(ctx, provider.ChatRequest{Messages: []provider.ChatMessage{visionProbeMessage()}, MaxTokens: 128, DisableThinking: true})
	if err != nil {
		return provider.ChatResponse{}, err
	}
	if strings.TrimSpace(resp.Content) == "" {
		return provider.ChatResponse{}, fmt.Errorf("模型未返回视觉探测响应")
	}
	return resp, nil
}
