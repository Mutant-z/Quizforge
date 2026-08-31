package config

import (
	"os"
	"strconv"
	"strings"
)

// Config 是 QuizTrace 后端统一配置，全部从环境变量读取（支持 .env 文件）。
type Config struct {
	Server      ServerConfig
	DB          DBConfig
	Upload      UploadConfig
	Worker      WorkerConfig
	Auth        AuthConfig
	Agent       AgentConfig
	RAG         RAGConfig
	Review      ReviewConfig
	WrongImport WrongImportConfig
}

type ServerConfig struct {
	Host        string
	Port        int
	Env         string // development | production
	FrontendDir string // 静态前端目录（可选）
}

type DBConfig struct {
	Path          string
	BusyTimeoutMs int
}

type UploadConfig struct {
	Root          string
	ChunkSizeMB   int
	MaxFileSizeMB int
	MaxPages      int
	AllowedTypes  []string
}

type WorkerConfig struct {
	ImportConcurrency        int
	LLMConcurrency           int
	EmbeddingConcurrency     int
	PollIntervalMs           int
	LeaseSeconds             int
	DirectoryConcurrency     int
	ExtractionConcurrency    int
	VisionMaxPages           int
	LocalInitialConcurrency  int
	RemoteInitialConcurrency int
	MaxVisionConcurrency     int
	VisionTimeoutSeconds     int
	CircuitBreakerSeconds    int
	NoProgressWarningSeconds int
	OCREnabled               bool
	OCRLanguage              string
	OCRBinaryPath            string
	OCRTimeoutSeconds        int
}

type AuthConfig struct {
	JWTSecret       string
	AccessTokenTTL  int    // 分钟
	RefreshTokenTTL int    // 天
	MasterKey       string // AES-GCM 加密 Provider API Key 用
	RateLimitPerMin int
}

type AgentConfig struct {
	MaxSteps      int
	ImportAgentV2 bool
}

type RAGConfig struct {
	FTSTopK    int
	VectorTopK int
	FinalTopK  int
}

type ReviewConfig struct {
	Scheduler string
}

// WrongImportConfig 错题导入 Agent 配置（单张图片校验与提取行为）。
type WrongImportConfig struct {
	MaxImageMB          int
	AutoExtract         bool
	MaxImageDimensionPx int
	AllowedImageTypes   []string
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Host:        getEnv("QT_SERVER_HOST", "0.0.0.0"),
			Port:        getEnvInt("QT_SERVER_PORT", 8080),
			Env:         getEnv("QT_ENV", "development"),
			FrontendDir: getEnv("QT_FRONTEND_DIR", ""),
		},
		DB: DBConfig{
			Path: getEnv("QT_DATABASE_PATH", "./data/app.db"),
			// 后台批量任务可能连续持有写锁数秒，默认值需要覆盖其单次写事务时长
			BusyTimeoutMs: getEnvInt("QT_DB_BUSY_TIMEOUT_MS", 15000),
		},
		Upload: UploadConfig{
			Root:          getEnv("QT_UPLOAD_ROOT", "./data"),
			ChunkSizeMB:   getEnvInt("QT_UPLOAD_CHUNK_SIZE_MB", 5),
			MaxFileSizeMB: getEnvInt("QT_UPLOAD_MAX_FILE_SIZE_MB", 2048),
			MaxPages:      getEnvInt("QT_UPLOAD_MAX_PAGES", 5000),
			AllowedTypes:  []string{"application/pdf", "text/plain", "text/markdown", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		},
		Worker: WorkerConfig{
			ImportConcurrency:        getEnvInt("QT_WORKER_IMPORT_CONCURRENCY", 1),
			LLMConcurrency:           getEnvInt("QT_WORKER_LLM_CONCURRENCY", 3),
			EmbeddingConcurrency:     getEnvInt("QT_WORKER_EMBEDDING_CONCURRENCY", 2),
			PollIntervalMs:           getEnvInt("QT_WORKER_POLL_INTERVAL_MS", 2000),
			LeaseSeconds:             getEnvInt("QT_WORKER_LEASE_SECONDS", 300),
			DirectoryConcurrency:     getEnvInt("QT_IMPORT_DIRECTORY_CONCURRENCY", 3),
			ExtractionConcurrency:    getEnvInt("QT_IMPORT_EXTRACTION_CONCURRENCY", 2),
			VisionMaxPages:           getEnvInt("QT_VISION_IMPORT_MAX_PAGES", 500),
			LocalInitialConcurrency:  getEnvInt("QT_IMPORT_LLM_LOCAL_INITIAL_CONCURRENCY", 2),
			RemoteInitialConcurrency: getEnvInt("QT_IMPORT_LLM_REMOTE_INITIAL_CONCURRENCY", 3),
			MaxVisionConcurrency:     getEnvInt("QT_IMPORT_LLM_MAX_CONCURRENCY", 3),
			VisionTimeoutSeconds:     getEnvInt("QT_IMPORT_VISION_TIMEOUT_SECONDS", 240),
			CircuitBreakerSeconds:    getEnvInt("QT_IMPORT_CIRCUIT_BREAKER_SECONDS", 60),
			NoProgressWarningSeconds: getEnvInt("QT_IMPORT_NO_PROGRESS_WARNING_SECONDS", 180),
			OCREnabled:               getEnvBool("QT_IMPORT_OCR_ENABLED", true),
			OCRLanguage:              getEnv("QT_IMPORT_OCR_LANG", "chi_sim+eng"),
			OCRBinaryPath:            getEnv("QT_IMPORT_TESSERACT_PATH", "/opt/homebrew/bin/tesseract"),
			OCRTimeoutSeconds:        getEnvInt("QT_IMPORT_OCR_TIMEOUT_SECONDS", 45),
		},
		Auth: AuthConfig{
			JWTSecret:       getEnv("QT_JWT_SECRET", "dev-secret-change-me"),
			AccessTokenTTL:  getEnvInt("QT_ACCESS_TOKEN_TTL_MIN", 120),
			RefreshTokenTTL: getEnvInt("QT_REFRESH_TOKEN_TTL_DAYS", 30),
			MasterKey:       getEnv("QT_API_KEY_MASTER_KEY", ""),
			RateLimitPerMin: getEnvInt("QT_RATE_LIMIT_PER_MIN", 120),
		},
		Agent: AgentConfig{
			MaxSteps:      getEnvInt("QT_AGENT_MAX_STEPS", 8),
			ImportAgentV2: getEnvBool("QT_IMPORT_AGENT_V2_ENABLED", true),
		},
		RAG: RAGConfig{
			FTSTopK:    getEnvInt("QT_RAG_FTS_TOP_K", 20),
			VectorTopK: getEnvInt("QT_RAG_VECTOR_TOP_K", 20),
			FinalTopK:  getEnvInt("QT_RAG_FINAL_TOP_K", 6),
		},
		Review: ReviewConfig{
			Scheduler: getEnv("QT_REVIEW_SCHEDULER", "simple_v1"),
		},
		WrongImport: WrongImportConfig{
			MaxImageMB:          getEnvInt("QT_WRONG_IMPORT_MAX_IMAGE_MB", 10),
			AutoExtract:         getEnvBool("QT_WRONG_IMPORT_AUTO_EXTRACT", true),
			MaxImageDimensionPx: getEnvInt("QT_WRONG_IMPORT_MAX_DIMENSION_PX", 2000),
			AllowedImageTypes:   []string{"image/png", "image/jpeg"},
		},
	}
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

func getEnvBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return parsed
}
