package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type serverConfig struct {
	Address       string
	APIOnly       bool
	Token         string
	Origins       []string
	MaxConcurrent int
}

func (c serverConfig) local() bool {
	host, _, err := net.SplitHostPort(c.Address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
func (c serverConfig) validate() error {
	if _, _, err := net.SplitHostPort(c.Address); err != nil {
		return fmt.Errorf("无效的监听地址：%w", err)
	}
	if !c.local() && len(c.Token) < 32 {
		return fmt.Errorf("非本机监听需要设置至少 32 字符的 ARXML_API_TOKEN")
	}
	if c.MaxConcurrent < 1 || c.MaxConcurrent > 64 {
		return fmt.Errorf("max-concurrent 必须在 1–64 之间")
	}
	if !c.APIOnly && !hasUI {
		return fmt.Errorf("headless 构建不包含前端，请使用 --api-only")
	}
	for _, raw := range c.Origins {
		origin := strings.TrimSpace(raw)
		if origin == "" {
			continue
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return fmt.Errorf("无效的 CORS Origin：%s（仅允许 scheme://host:port）", origin)
		}
	}
	return nil
}
func handler() http.Handler {
	return newHandler(serverConfig{Address: "127.0.0.1:8765", APIOnly: !hasUI, MaxConcurrent: 4})
}
func newHandler(c serverConfig) http.Handler {
	mux := http.NewServeMux()
	allowed := map[string]bool{}
	for _, origin := range c.Origins {
		if origin = strings.TrimSpace(origin); origin != "" {
			allowed[origin] = true
		}
	}
	slots := make(chan struct{}, c.MaxConcurrent)
	expected := sha256.Sum256([]byte(c.Token))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if c.Token != "" {
			provided, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			digest := sha256.Sum256([]byte(provided))
			if !ok || subtle.ConstantTimeCompare(expected[:], digest[:]) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="arxml-studio"`)
				reply(w, 401, map[string]string{"error": "需要有效的 Bearer token"})
				return
			}
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			w.Header().Set("Retry-After", "1")
			reply(w, 429, map[string]string{"error": "服务繁忙，请稍后重试"})
			return
		}
		api(w, r)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			reply(w, 405, map[string]string{"error": "仅支持 GET/HEAD"})
			return
		}
		reply(w, 200, map[string]string{"status": "ok", "version": version})
	})
	if !c.APIOnly {
		mux.Handle("/", staticHandler())
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			reply(w, 404, map[string]string{"error": "API-only 服务，不提供页面"})
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rec := &statusWriter{ResponseWriter: w, status: 200}
		defer func() {
			slog.Info("http_request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(started))
		}()
		w = rec
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; frame-ancestors 'none'")
		if c.local() {
			host, _, err := net.SplitHostPort(r.Host)
			if err != nil {
				host = r.Host
			}
			ip := net.ParseIP(host)
			if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
				reply(w, 403, map[string]string{"error": "仅接受本机访问"})
				return
			}
		}
		origin := r.Header.Get("Origin")
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if origin != "" {
			w.Header().Add("Vary", "Origin")
			if origin != scheme+"://"+r.Host && !allowed[origin] {
				reply(w, 403, map[string]string{"error": "Origin 未获允许"})
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		if r.Method == "OPTIONS" && strings.HasPrefix(r.URL.Path, "/api/") {
			if origin == "" || r.Header.Get("Access-Control-Request-Method") != "POST" {
				reply(w, 400, map[string]string{"error": "无效的预检请求"})
				return
			}
			for _, h := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
				h = strings.ToLower(strings.TrimSpace(h))
				if h != "" && h != "authorization" && h != "content-type" {
					reply(w, 403, map[string]string{"error": "预检请求头未获允许"})
					return
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.WriteHeader(204)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
