package main

import (
	"arxml-studio/internal/arxml"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var version = "0.3.0"

type inputFile struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func (f inputFile) parse() (*arxml.Document, error) {
	data, e := base64.StdEncoding.DecodeString(f.Data)
	if e != nil {
		return nil, fmt.Errorf("无效的文件编码")
	}
	return arxml.Parse(data, f.Name)
}

type request struct {
	inputFile
	File     inputFile      `json:"file"`
	Before   inputFile      `json:"before"`
	After    inputFile      `json:"after"`
	Path     string         `json:"path"`
	Settings arxml.Settings `json:"settings"`
}

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func api(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		reply(w, 405, map[string]string{"error": "仅支持 POST"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, arxml.MaxBytes*3)
	defer r.Body.Close()
	var req request
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if e := decoder.Decode(&req); e != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(e, &tooLarge) {
			reply(w, 413, map[string]string{"error": "请求超过 60 MiB 限制"})
		} else {
			reply(w, 400, map[string]string{"error": "无效的 JSON 请求"})
		}
		return
	}
	if e := decoder.Decode(new(any)); e != io.EOF {
		reply(w, 400, map[string]string{"error": "请求只能包含一个 JSON 对象"})
		return
	}
	var result any
	var err error
	switch r.URL.Path {
	case "/api/parse":
		result, err = req.inputFile.parse()
	case "/api/compare":
		var a, b *arxml.Document
		a, err = req.Before.parse()
		if err == nil {
			b, err = req.After.parse()
		}
		if err == nil {
			result = arxml.Compare(a, b)
		}
	case "/api/settings", "/api/export", "/api/export/zip":
		var doc *arxml.Document
		doc, err = req.File.parse()
		if err == nil {
			s := doc.Service(req.Path)
			if s == nil {
				err = fmt.Errorf("未找到所选服务")
			} else if r.URL.Path == "/api/settings" {
				result = arxml.Template(s)
			} else {
				result, err = arxml.Export(doc, req.Path, req.Settings)
			}
		}
	default:
		reply(w, 404, map[string]string{"error": "未知 API"})
		return
	}
	if err != nil {
		reply(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if r.URL.Path == "/api/export/zip" {
		bundle := result.(arxml.Bundle)
		data, err := base64.StdEncoding.DecodeString(bundle.Data)
		if err != nil {
			reply(w, 500, map[string]string{"error": "无法编码导出文件"})
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", bundle.Filename))
		_, _ = w.Write(data)
		return
	}
	reply(w, 200, result)
}
func readFile(path string) (*arxml.Document, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, arxml.MaxBytes+1))
	if e != nil {
		return nil, e
	}
	return arxml.Parse(data, filepath.Base(path))
}
func run() error {
	listen := flag.String("listen", os.Getenv("ARXML_LISTEN"), "监听地址，例如 0.0.0.0:8080；默认 127.0.0.1:8765")
	apiOnly := flag.Bool("api-only", !hasUI, "仅提供 HTTP API")
	origins := flag.String("cors-origins", os.Getenv("ARXML_CORS_ORIGINS"), "允许的前端 Origin，逗号分隔")
	concurrency := flag.Int("max-concurrent", 4, "最大并发 API 请求数")
	healthcheck := flag.String("healthcheck", "", "探测健康检查 URL 后退出")
	port := flag.Int("port", 8765, "本地 HTTP 端口（0 为自动分配）")
	exportPath := flag.String("export", "", "将 ARXML 导出为 JSON")
	comparePath := flag.String("compare", "", "比较的基准 ARXML（与 --with 一起使用）")
	withPath := flag.String("with", "", "比较的新版本 ARXML")
	bundlePath := flag.String("bundle", "", "导出 CommonAPI/vsomeip ZIP 的 ARXML")
	service := flag.String("service", "", "完整服务路径")
	settings := flag.String("settings", "", "导出参数 JSON")
	output := flag.String("output", "", "输出文件")
	showVersion := flag.Bool("version", false, "显示版本")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("不支持的位置参数：%s", strings.Join(flag.Args(), " "))
	}
	if *healthcheck != "" {
		client := &http.Client{Timeout: 3 * time.Second}
		response, err := client.Get(*healthcheck)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return fmt.Errorf("healthcheck: HTTP %d", response.StatusCode)
		}
		return nil
	}
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	modes := 0
	for _, v := range []string{*exportPath, *comparePath, *bundlePath} {
		if v != "" {
			modes++
		}
	}
	if modes > 1 {
		return fmt.Errorf("--export、--compare、--bundle 不能同时使用")
	}
	if modes > 0 {
		path := *exportPath
		if *comparePath != "" {
			path = *comparePath
		}
		if *bundlePath != "" {
			path = *bundlePath
		}
		doc, e := readFile(path)
		if e != nil {
			return e
		}
		var result any = doc
		var data []byte
		if *comparePath != "" {
			if *withPath == "" {
				return fmt.Errorf("--compare 需要 --with")
			}
			other, e := readFile(*withPath)
			if e != nil {
				return e
			}
			result = arxml.Compare(doc, other)
		}
		if *bundlePath != "" {
			if *settings == "" || *service == "" || *output == "" {
				return fmt.Errorf("--bundle 需要 --settings、--service 和 --output")
			}
			raw, e := os.ReadFile(*settings)
			if e != nil {
				return e
			}
			var cfg arxml.Settings
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.UseNumber()
			if e = decoder.Decode(&cfg); e != nil {
				return e
			}
			bundle, e := arxml.Export(doc, *service, cfg)
			if e != nil {
				return e
			}
			data, e = base64.StdEncoding.DecodeString(bundle.Data)
			if e != nil {
				return e
			}
		} else {
			data, e = json.MarshalIndent(result, "", "  ")
			if e != nil {
				return e
			}
			data = append(data, '\n')
		}
		if *output != "" {
			return os.WriteFile(*output, data, 0644)
		}
		_, e = os.Stdout.Write(data)
		return e
	}
	if *withPath != "" || *settings != "" || *service != "" || *output != "" {
		return fmt.Errorf("请指定 --export、--compare 或 --bundle")
	}
	address := *listen
	if address == "" {
		address = fmt.Sprintf("127.0.0.1:%d", *port)
	}
	cfg := serverConfig{Address: address, APIOnly: *apiOnly, Token: os.Getenv("ARXML_API_TOKEN"), Origins: strings.Split(*origins, ","), MaxConcurrent: *concurrency}
	if err := cfg.validate(); err != nil {
		return err
	}
	listener, e := net.Listen("tcp", address)
	if e != nil {
		return e
	}
	server := &http.Server{Handler: newHandler(cfg), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
	}()
	fmt.Printf("ARXML Studio %s · HTTP service\nhttp://%s\n按 Ctrl+C 退出\n", version, listener.Addr())
	if e = server.Serve(listener); e != http.ErrServerClosed {
		return e
	}
	<-stopped
	return nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
