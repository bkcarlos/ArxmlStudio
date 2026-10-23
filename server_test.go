package main

import (
	"arxml-studio/internal/arxml"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const testToken = "0123456789abcdef0123456789abcdef"

func serviceConfig() serverConfig {
	return serverConfig{Address: "0.0.0.0:8080", APIOnly: true, Token: testToken, Origins: []string{"https://studio.example"}, MaxConcurrent: 1}
}
func TestServiceConfig(t *testing.T) {
	c := serviceConfig()
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	c.Token = ""
	if c.validate() == nil {
		t.Fatal("anonymous remote listener allowed")
	}
	c.Address = "127.0.0.1:0"
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	c.Origins = []string{"*"}
	if c.validate() == nil {
		t.Fatal("wildcard origin accepted")
	}
	c.Origins = nil
	c.MaxConcurrent = 0
	if c.validate() == nil {
		t.Fatal("invalid concurrency accepted")
	}
}
func TestServiceAuthCORSAndHeadless(t *testing.T) {
	h := newHandler(serviceConfig())
	for _, tc := range []struct {
		name, method, path, origin, token string
		want                              int
	}{
		{"health", "GET", "/health", "", "", 200},
		{"no UI", "GET", "/", "", "", 404},
		{"no auth", "POST", "/api/parse", "", "", 401},
		{"bad auth", "POST", "/api/parse", "", "wrong", 401},
		{"authenticated", "POST", "/api/parse", "", testToken, 200},
		{"allowed origin", "POST", "/api/parse", "https://studio.example", testToken, 200},
		{"blocked origin", "POST", "/api/parse", "https://other.example", testToken, 403},
		{"preflight", "OPTIONS", "/api/parse", "https://studio.example", "", 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(inputFile{Name: "empty.arxml", Data: base64.StdEncoding.EncodeToString([]byte("<AUTOSAR/>"))})
			r := httptest.NewRequest(tc.method, "http://api.example:8080"+tc.path, bytes.NewReader(data))
			r.Header.Set("Origin", tc.origin)
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			if tc.method == "OPTIONS" {
				r.Header.Set("Access-Control-Request-Method", "POST")
				r.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
			if tc.origin == "https://studio.example" && w.Header().Get("Access-Control-Allow-Origin") != tc.origin {
				t.Fatal("CORS response missing")
			}
		})
	}
}
func TestDirectZIP(t *testing.T) {
	data, e := os.ReadFile("examples/vehicle-v1.arxml")
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("examples/export-settings.json")
	if e != nil {
		t.Fatal(e)
	}
	var cfg arxml.Settings
	if e = json.Unmarshal(raw, &cfg); e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(map[string]any{"file": inputFile{Name: "demo", Data: base64.StdEncoding.EncodeToString(data)}, "path": "/Vehicle/VehicleService", "settings": cfg})
	r := httptest.NewRequest("POST", "http://localhost/api/export/zip", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler().ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/zip" || !bytes.HasPrefix(w.Body.Bytes(), []byte("PK")) {
		t.Fatalf("invalid ZIP response %d %s", w.Code, w.Body.String())
	}
}

type startedReader struct {
	io.Reader
	started chan struct{}
	first   bool
}

func (r *startedReader) Read(p []byte) (int, error) {
	if !r.first {
		r.first = true
		close(r.started)
	}
	return r.Reader.Read(p)
}
func TestConcurrencyLimit(t *testing.T) {
	h := newHandler(serviceConfig())
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	body := &startedReader{Reader: reader, started: make(chan struct{})}
	r := httptest.NewRequest("POST", "http://api.example/api/parse", body)
	r.Header.Set("Authorization", "Bearer "+testToken)
	done := make(chan struct{})
	go func() { defer close(done); h.ServeHTTP(httptest.NewRecorder(), r) }()
	<-body.started
	second := httptest.NewRequest("POST", "http://api.example/api/parse", strings.NewReader("{}"))
	second.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, second)
	if w.Code != 429 {
		t.Errorf("expected backpressure, got %d", w.Code)
	}
	writer.Close()
	<-done
	third := httptest.NewRequest("POST", "http://api.example/api/parse", strings.NewReader("{}"))
	third.Header.Set("Authorization", "Bearer "+testToken)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, third)
	if w.Code != 400 {
		t.Fatal("request slot was not released")
	}
}
