package router

import (
	"database/sql"

	"github.com/quiztrace/quiztrace/internal/security"
)

// Deps 汇聚服务依赖，由 main 组装后注入路由。
type Deps struct {
	DB    *sql.DB
	Token *security.TokenManager
	Crypt *security.AESCrypt
}
