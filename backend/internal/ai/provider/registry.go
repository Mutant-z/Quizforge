package provider

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/quiztrace/quiztrace/internal/observability"
	"github.com/quiztrace/quiztrace/internal/security"
)

// ProviderType 定义模型类型常量
const (
	TypeLLM       = "llm"
	TypeEmbedding = "embedding"
	TypeRerank    = "rerank"
)

// StoredProvider 数据库中的 Provider 记录。
type StoredProvider struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	Type           string `json:"provider_type"` // llm | embedding | rerank
	Name           string `json:"name"`
	Protocol       string `json:"protocol"`
	BaseURL        string `json:"base_url"`
	APIKeyMasked   string `json:"api_key_masked"`
	ModelName      string `json:"model_name"`
	IsDefault      bool   `json:"is_default"`
	SupportsVision bool   `json:"supports_vision"`
}

// Registry 从 DB 读取 Provider 配置并构造实例。
type Registry struct {
	db    *sql.DB
	crypt *security.AESCrypt
}

func NewRegistry(db *sql.DB, crypt *security.AESCrypt) *Registry {
	return &Registry{db: db, crypt: crypt}
}

// List 根据类型筛选当前用户的 Provider（若 providerType 为空则列出所有）。
func (r *Registry) List(ctx context.Context, providerType string) ([]StoredProvider, error) {
	userID := observability.UserID(ctx)
	query := `
		SELECT id, COALESCE(user_id, 0), COALESCE(provider_type, 'llm'), name, protocol, base_url, api_key_encrypted, 
		       COALESCE(model_name, chat_model, embedding_model, rerank_model, ''), is_default, COALESCE(supports_vision, 0)
		FROM ai_providers
		WHERE user_id = ?`
	args := []interface{}{userID}
	if providerType != "" {
		query += " AND provider_type = ?"
		args = append(args, providerType)
	}
	query += " ORDER BY is_default DESC, id DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StoredProvider{}
	for rows.Next() {
		var p StoredProvider
		var enc string
		var isDefault, supportsVision int
		if err := rows.Scan(&p.ID, &p.UserID, &p.Type, &p.Name, &p.Protocol, &p.BaseURL, &enc, &p.ModelName, &isDefault, &supportsVision); err != nil {
			return nil, err
		}
		p.IsDefault = isDefault == 1
		p.SupportsVision = supportsVision == 1
		key, err := r.crypt.Decrypt(enc)
		if err == nil {
			p.APIKeyMasked = security.MaskKey(key)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Save 保存指定类型的 Provider（关联当前登录用户）。
func (r *Registry) Save(ctx context.Context, providerType, name, protocol, baseURL, apiKey, modelName string, isDefault bool, vision ...bool) (*StoredProvider, error) {
	userID := observability.UserID(ctx)
	if providerType == "" {
		providerType = TypeLLM
	}
	enc, err := r.crypt.Encrypt(apiKey)
	if err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	def := 0
	supportsVision := 0
	if len(vision) > 0 && vision[0] {
		supportsVision = 1
	}
	if isDefault {
		def = 1
		// 仅取消当前用户同类型的默认标记
		_, _ = tx.ExecContext(ctx, `UPDATE ai_providers SET is_default = 0 WHERE user_id = ? AND provider_type = ?`, userID, providerType)
	}

	res, err := tx.ExecContext(ctx, `
			INSERT INTO ai_providers (user_id, provider_type, name, protocol, base_url, api_key_encrypted, model_name, chat_model, is_default, supports_vision)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, providerType, name, protocol, baseURL, enc, modelName, modelName, def, supportsVision)
	if err != nil {
		return nil, fmt.Errorf("save provider: %w", err)
	}

	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.Get(ctx, id)
}

// Update 更新当前用户的 Provider 基础配置。
func (r *Registry) Update(ctx context.Context, id int64, name, baseURL, apiKey, modelName string, isDefault bool, vision ...bool) error {
	userID := observability.UserID(ctx)
	p, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if p.UserID != userID && userID != 0 {
		return fmt.Errorf("permission denied: provider does not belong to user")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if isDefault {
		_, _ = tx.ExecContext(ctx, `UPDATE ai_providers SET is_default = 0 WHERE user_id = ? AND provider_type = ?`, p.UserID, p.Type)
	}

	defInt := 0
	if isDefault {
		defInt = 1
	}
	supportsVision := p.SupportsVision
	if len(vision) > 0 {
		supportsVision = vision[0]
	}
	visionInt := 0
	if supportsVision {
		visionInt = 1
	}

	if apiKey != "" {
		enc, err := r.crypt.Encrypt(apiKey)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE ai_providers 
			SET name = ?, base_url = ?, api_key_encrypted = ?, model_name = ?, chat_model = ?, is_default = ?, supports_vision = ?, updated_at = datetime('now')
			WHERE id = ? AND user_id = ?`, name, baseURL, enc, modelName, modelName, defInt, visionInt, id, p.UserID)
		if err != nil {
			return err
		}
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE ai_providers 
			SET name = ?, base_url = ?, model_name = ?, chat_model = ?, is_default = ?, supports_vision = ?, updated_at = datetime('now')
			WHERE id = ? AND user_id = ?`, name, baseURL, modelName, modelName, defInt, visionInt, id, p.UserID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// SetDefault 将指定 Provider 设为该用户在该类型的默认 Provider。
func (r *Registry) SetDefault(ctx context.Context, id int64) error {
	userID := observability.UserID(ctx)
	p, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if p.UserID != userID && userID != 0 {
		return fmt.Errorf("permission denied: provider does not belong to user")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE ai_providers SET is_default = 0 WHERE user_id = ? AND provider_type = ?`, p.UserID, p.Type); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ai_providers SET is_default = 1 WHERE id = ? AND user_id = ?`, id, p.UserID); err != nil {
		return err
	}
	return tx.Commit()
}

// Delete 删除当前用户的 Provider。
func (r *Registry) Delete(ctx context.Context, id int64) error {
	userID := observability.UserID(ctx)
	res, err := r.db.ExecContext(ctx, `DELETE FROM ai_providers WHERE id = ? AND (user_id = ? OR ? = 0)`, id, userID, userID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("provider not found or no permission")
	}
	return nil
}

// Get 查询单个 Provider 记录。
func (r *Registry) Get(ctx context.Context, id int64) (*StoredProvider, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(user_id, 0), COALESCE(provider_type, 'llm'), name, protocol, base_url, api_key_encrypted, 
		       COALESCE(model_name, chat_model, embedding_model, rerank_model, ''), is_default, COALESCE(supports_vision, 0)
		FROM ai_providers WHERE id = ?`, id)
	var p StoredProvider
	var enc string
	var isDefault, supportsVision int
	if err := row.Scan(&p.ID, &p.UserID, &p.Type, &p.Name, &p.Protocol, &p.BaseURL, &enc, &p.ModelName, &isDefault, &supportsVision); err != nil {
		return nil, err
	}
	p.IsDefault = isDefault == 1
	p.SupportsVision = supportsVision == 1
	key, err := r.crypt.Decrypt(enc)
	if err == nil {
		p.APIKeyMasked = security.MaskKey(key)
	}
	return &p, nil
}

// Default 返回指定类型（llm, embedding, rerank）的默认 Provider 配置（含解密 Key）。
// 优先按当前用户的默认配置 -> 当前用户的首个配置 -> 系统全局(user_id=0)默认配置 -> 系统全局首个配置 回退。
func (r *Registry) Default(ctx context.Context, providerType string) (*Config, error) {
	if providerType == "" {
		providerType = TypeLLM
	}
	userID := observability.UserID(ctx)

	var id int64
	var pType, name, protocol, baseURL, enc, modelName string
	var supportsVision int

	scanQuery := func(query string, args ...interface{}) error {
		row := r.db.QueryRowContext(ctx, query, args...)
		return row.Scan(&id, &pType, &name, &protocol, &baseURL, &enc, &modelName, &supportsVision)
	}

	found := false

	// 1. 如果有当前用户，查询该用户设置的 default
	if userID > 0 {
		err := scanQuery(`
			SELECT id, COALESCE(provider_type, 'llm'), name, protocol, base_url, api_key_encrypted, 
			       COALESCE(model_name, chat_model, embedding_model, rerank_model, ''), COALESCE(supports_vision, 0)
			FROM ai_providers 
			WHERE user_id = ? AND provider_type = ? AND is_default = 1 
			ORDER BY id DESC LIMIT 1`, userID, providerType)
		if err == nil {
			found = true
		} else if err != sql.ErrNoRows {
			return nil, err
		}

		// 2. 该用户未设 default，回退该用户的首个可用配置
		if !found {
			err := scanQuery(`
				SELECT id, COALESCE(provider_type, 'llm'), name, protocol, base_url, api_key_encrypted, 
				       COALESCE(model_name, chat_model, embedding_model, rerank_model, ''), COALESCE(supports_vision, 0)
				FROM ai_providers 
				WHERE user_id = ? AND provider_type = ? 
				ORDER BY id ASC LIMIT 1`, userID, providerType)
			if err == nil {
				found = true
			} else if err != sql.ErrNoRows {
				return nil, err
			}
		}
	}

	// 3. 回退系统全局(user_id = 0)默认配置
	if !found {
		err := scanQuery(`
			SELECT id, COALESCE(provider_type, 'llm'), name, protocol, base_url, api_key_encrypted, 
			       COALESCE(model_name, chat_model, embedding_model, rerank_model, ''), COALESCE(supports_vision, 0)
			FROM ai_providers 
			WHERE user_id = 0 AND provider_type = ? AND is_default = 1 
			ORDER BY id DESC LIMIT 1`, providerType)
		if err == nil {
			found = true
		} else if err != sql.ErrNoRows {
			return nil, err
		}
	}

	// 4. 回退系统全局首个可用配置
	if !found {
		err := scanQuery(`
			SELECT id, COALESCE(provider_type, 'llm'), name, protocol, base_url, api_key_encrypted, 
			COALESCE(model_name, chat_model, embedding_model, rerank_model, ''), COALESCE(supports_vision, 0)
			FROM ai_providers 
			WHERE user_id = 0 AND provider_type = ? 
			ORDER BY id ASC LIMIT 1`, providerType)
		if err == nil {
			found = true
		} else if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no provider configured for type: %s", providerType)
		} else {
			return nil, err
		}
	}

	key, err := r.crypt.Decrypt(enc)
	if err != nil {
		return nil, fmt.Errorf("decrypt provider key: %w", err)
	}
	return &Config{
		ID: id, Name: name, Protocol: protocol, BaseURL: baseURL, APIKey: key,
		ChatModel: modelName, EmbeddingModel: modelName, RerankModel: modelName, TimeoutSec: 180, SupportsVision: supportsVision == 1,
	}, nil
}

// NewLLM 根据存储配置构造 LLM Provider。
func (r *Registry) NewLLM(ctx context.Context, storedID int64) (LLMProvider, error) {
	var p *StoredProvider
	var err error
	if storedID > 0 {
		p, err = r.Get(ctx, storedID)
	} else {
		cfg, err2 := r.Default(ctx, TypeLLM)
		if err2 != nil {
			return nil, err2
		}
		return NewFromConfig(*cfg)
	}
	if err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx, `SELECT api_key_encrypted FROM ai_providers WHERE id = ?`, p.ID)
	var enc string
	if err := row.Scan(&enc); err != nil {
		return nil, err
	}
	key, err := r.crypt.Decrypt(enc)
	if err != nil {
		return nil, err
	}
	cfg := Config{
		Name: p.Name, Protocol: p.Protocol, BaseURL: p.BaseURL, APIKey: key,
		ChatModel: p.ModelName, TimeoutSec: 180, SupportsVision: p.SupportsVision,
	}
	return NewFromConfig(cfg)
}

// NewFromConfig 按协议构造 LLM Provider。
func NewFromConfig(cfg Config) (LLMProvider, error) {
	switch cfg.Protocol {
	case "anthropic":
		return NewAnthropicProvider(cfg), nil
	case "openai", "":
		return NewOpenAIProvider(cfg), nil
	default:
		return nil, fmt.Errorf("unknown protocol: %s", cfg.Protocol)
	}
}
