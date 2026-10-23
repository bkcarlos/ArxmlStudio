package main

import (
	"arxml-studio/internal/arxml"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestEmbeddedAssets(t *testing.T) {
	if !hasUI {
		t.Skip("headless binary intentionally excludes UI")
	}
	for _, path := range []string{"/", "/examples/vehicle-v1.arxml", "/health"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8765"+path, nil)
		w := httptest.NewRecorder()
		handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d", path, w.Code)
		}
		if path == "/" && !strings.Contains(w.Body.String(), "/assets/") {
			t.Fatal("Vue build not embedded")
		}
	}
}
func TestAPI(t *testing.T) {
	data, e := os.ReadFile("examples/vehicle-v1.arxml")
	if e != nil {
		t.Fatal(e)
	}
	f := inputFile{Name: "demo.arxml", Data: base64.StdEncoding.EncodeToString(data)}
	settingsData, e := os.ReadFile("examples/export-settings.json")
	if e != nil {
		t.Fatal(e)
	}
	var cfg arxml.Settings
	_ = json.Unmarshal(settingsData, &cfg)
	for _, test := range []struct {
		path string
		body any
		want int
	}{{"parse", f, 200}, {"compare", map[string]any{"before": f, "after": f}, 200}, {"settings", map[string]any{"file": f, "path": "/Vehicle/VehicleService"}, 200}, {"export", map[string]any{"file": f, "path": "/Vehicle/VehicleService", "settings": cfg}, 200}, {"parse", inputFile{Data: "bad"}, 400}, {"settings", map[string]any{"file": f, "path": "missing"}, 400}} {
		t.Run(test.path, func(t *testing.T) {
			body, _ := json.Marshal(test.body)
			r := httptest.NewRequest("POST", "http://127.0.0.1:8765/api/"+test.path, bytes.NewReader(body))
			w := httptest.NewRecorder()
			handler().ServeHTTP(w, r)
			if w.Code != test.want {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
		})
	}
}
func TestAPIRejectsInvalidRequests(t *testing.T) {
	for _, body := range []string{"[]", "{", "{}{}", "{\"settings\":{\"members\":42}}"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "http://127.0.0.1:8765/api/parse", strings.NewReader(body))
		handler().ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("invalid JSON accepted: %s", body)
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "http://127.0.0.1:8765/api/parse", strings.NewReader("{}"))
	r.Header.Set("Origin", "https://untrusted.example")
	handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-site origin accepted")
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "http://untrusted.example/", nil)
	handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("foreign host accepted")
	}
}
