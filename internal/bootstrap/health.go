// Package bootstrap 应用启动编排层
package bootstrap

import (
	"encoding/json"
	"net/http"

	"aiProject/internal/infrastructure/database"
)

// healthResponse 健康检查响应
type healthResponse struct {
	Status string            `json:"status"`
	Ready  bool              `json:"ready,omitempty"`
	Checks map[string]string `json:"checks,omitempty"`
}

// registerHealthEndpoints 在默认 ServeMux 上注册健康检查端点。
// 注册在中间件链之外，使探针请求不受认证 / 限流 / CORS 影响（K8s liveness/readiness 探针专用）。
//   - /healthz 存活探针：进程存活即返回 200
//   - /readyz  就绪探针：依赖（数据库）就绪才返回 200，否则 503
func registerHealthEndpoints() {
	// 存活探针：只要进程能响应即视为存活
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeHealth(w, http.StatusOK, healthResponse{Status: "ok"})
	})

	// 就绪探针：检查关键依赖是否就绪
	http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{}

		dbReady := database.GetDB() != nil
		if dbReady {
			checks["database"] = "ok"
		} else {
			checks["database"] = "unavailable"
		}

		// 缓存为可选依赖：未启用(noop)视为正常，启用但连不上视为降级(degraded)但不影响就绪
		switch {
		case appCache == nil, appCache.Backend() == "noop":
			checks["cache"] = "disabled"
		case appCache.Available():
			checks["cache"] = "ok"
		default:
			checks["cache"] = "degraded"
		}

		// 就绪判定：数据库为硬依赖
		ready := dbReady
		code := http.StatusOK
		status := "ready"
		if !ready {
			code = http.StatusServiceUnavailable
			status = "not_ready"
		}
		writeHealth(w, code, healthResponse{Status: status, Ready: ready, Checks: checks})
	})
}

func writeHealth(w http.ResponseWriter, code int, resp healthResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}
