package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Server mc-starter-server 实例
type Server struct {
	config  *ServerConfig
	store   PackStoreIface
	httpSrv *http.Server
}

// NewServer 创建 Server 实例
func NewServer(cfg *ServerConfig) (*Server, error) {
	store, err := NewStore(cfg, cfg.Storage.StoreType)
	if err != nil {
		return nil, fmt.Errorf("初始化存储后端失败: %w", err)
	}

	s := &Server{
		config: cfg,
		store:  store,
	}
	return s, nil
}

// Start 启动 HTTP 服务
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// 注册路由
	s.registerRoutes(mux)

	s.httpSrv = &http.Server{
		Addr:         s.config.ListenAddr(),
		Handler:      s.withMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 优雅关闭
	go s.gracefulShutdown()

	log.Printf("mc-starter-server 启动于 %s", s.config.ListenAddr())

	if s.config.Server.TLSEnabled {
		return s.httpSrv.ListenAndServeTLS(s.config.Server.TLSCert, s.config.Server.TLSKey)
	}
	return s.httpSrv.ListenAndServe()
}

// registerRoutes 注册所有路由
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// 客户端端点（可选 token 认证）
	mux.HandleFunc("GET /api/v1/ping", s.requireClientToken(s.handlePing))
	mux.HandleFunc("GET /api/v1/packs", s.requireClientToken(s.handleListPacks))
	mux.HandleFunc("GET /api/v1/packs/{name}", s.requireClientToken(s.handleGetPack))
	mux.HandleFunc("GET /api/v1/packs/{name}/update", s.requireClientToken(s.handleGetUpdate))
	mux.HandleFunc("GET /api/v1/packs/{name}/files/{hash}", s.requireClientToken(s.handleFileDownload))
	mux.HandleFunc("GET /api/v1/packs/{name}/download", s.requireClientToken(s.handleFullDownload))
	mux.HandleFunc("POST /api/v1/packs/{name}/crash-report", s.requireClientToken(s.handleCrashReport))

	// P6 频道端点（客户端和管理端均可查询）
	mux.HandleFunc("GET /api/v1/packs/{name}/channels", s.requireClientToken(s.handleListChannels))

	// 管理端端点（需认证）
	mux.HandleFunc("POST /api/v1/admin/packs", s.requireAdmin(s.handleCreatePack))
	mux.HandleFunc("DELETE /api/v1/admin/packs/{name}", s.requireAdmin(s.handleDeletePack))
	mux.HandleFunc("GET /api/v1/admin/packs/{name}/config", s.requireAdmin(s.handleGetPackConfig))
	mux.HandleFunc("PUT /api/v1/admin/packs/{name}/config", s.requireAdmin(s.handleUpdatePackConfig))
	mux.HandleFunc("POST /api/v1/admin/packs/{name}/import", s.requireAdmin(s.handleImportPack))
	mux.HandleFunc("POST /api/v1/admin/packs/{name}/publish", s.requireAdmin(s.handlePublishPack))
	mux.HandleFunc("GET /api/v1/admin/packs/{name}/versions", s.requireAdmin(s.handleListVersions))
	mux.HandleFunc("DELETE /api/v1/admin/packs/{name}/versions/{ver}", s.requireAdmin(s.handleDeleteVersion))
	// P6 管理端频道端点
	mux.HandleFunc("POST /api/v1/admin/packs/{name}/channels", s.requireAdmin(s.handleCreateChannel))
	mux.HandleFunc("DELETE /api/v1/admin/packs/{name}/channels/{channel}", s.requireAdmin(s.handleDeleteChannel))
}

// withMiddleware 包装通用的 HTTP 中间件
func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("→ %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		// CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// requireAdmin 管理端点认证中间件
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.config.Auth.Enabled {
			next(w, r)
			return
		}

		token := extractBearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "缺少认证 token")
			return
		}

		// 常量时间比较防时序攻击
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.config.Auth.AdminToken)) != 1 {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "token 无效")
			return
		}

		next(w, r)
	}
}

// requireClientToken 客户端端点认证中间件
// 仅在配置中启用 ClientRequireToken 时生效
func (s *Server) requireClientToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.config.Auth.Enabled || !s.config.Auth.ClientRequireToken {
			next(w, r)
			return
		}

		token := extractBearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "缺少客户端 token")
			return
		}

		// 客户端 token 与 admin token 相同（身份一致）
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.config.Auth.AdminToken)) != 1 {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "token 无效")
			return
		}

		next(w, r)
	}
}

// gracefulShutdown 监听信号做优雅关闭
func (s *Server) gracefulShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("正在关闭服务...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.httpSrv.Shutdown(ctx); err != nil {
		log.Printf("关闭失败: %v", err)
	}
}

// ============================================================
// 辅助函数
// ============================================================

// writeJSON 写入 JSON 响应
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError 写入 JSON 错误响应
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

// extractBearerToken 从 Authorization 头提取 Bearer token
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(auth[7:])
}
