package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func testHandler(t *testing.T, upstream http.HandlerFunc) (http.Handler, string) {
	t.Helper()
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newHandler(target)
	if err != nil {
		t.Fatal(err)
	}
	return handler, target.Host
}

func TestAllOperations(t *testing.T) {
	type received struct {
		method, path, query, host, body string
		header                          http.Header
	}
	requests := make(chan received, 1)
	handler, host := testHandler(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		requests <- received{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.Host, string(body), r.Header.Clone()}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream", "preserved")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `{"success":true,"body":{"test":1}}`)
	})
	doc, err := openapi3.NewLoader().LoadFromData(specification)
	if err != nil {
		t.Fatal(err)
	}
	parameter := regexp.MustCompile(`\{[^}]+\}`)
	count := 0
	for path, item := range doc.Paths.Map() {
		for method, operation := range item.Operations() {
			count++
			t.Run(operation.OperationID, func(t *testing.T) {
				encodedPath := parameter.ReplaceAllString(path, "sample%2Fvalue%20%2B")
				query := "a=1&a=2&encoded=a%2Bb+space"
				body := "name=A+B&action=0&value=%2F"
				r := httptest.NewRequest(method, "http://localhost/api"+encodedPath+"?"+query, strings.NewReader(body))
				r.Header.Set("Authorization", "Bearer test-token-not-real")
				r.Header.Set("X-Force-Org-Id", "test-org")
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				r.Header.Set("Idempotency-Key", "test-key")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != http.StatusMultiStatus {
					t.Fatalf("%s %s：status=%d body=%s", method, encodedPath, w.Code, w.Body.String())
				}
				got := <-requests
				if got.method != method || got.path != encodedPath || got.query != query || got.host != host || got.body != body {
					t.Errorf("轉送內容不符：%+v", got)
				}
				for _, name := range []string{"Authorization", "X-Force-Org-Id", "Content-Type", "Idempotency-Key"} {
					if got.header.Get(name) != r.Header.Get(name) {
						t.Errorf("標頭 %s 不符", name)
					}
				}
				if w.Header().Get("X-Upstream") != "preserved" || w.Header().Get("Content-Type") != "application/json" || w.Body.String() != `{"success":true,"body":{"test":1}}` {
					t.Errorf("上游回應被改寫：%v %s", w.Header(), w.Body.String())
				}
			})
		}
	}
	if count != 58 {
		t.Fatalf("預期 58 個操作，實際 %d", count)
	}
}

func TestUnknownRoutesAndMethods(t *testing.T) {
	handler, _ := testHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("不應呼叫上游")
		w.WriteHeader(http.StatusInternalServerError)
	})
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/api/unknown", 404},
		{"POST", "/api/unknown", 404},
		{"GET", "/api", 404},
		{"GET", "/api/", 404},
		{"PATCH", "/api/profiles", 405},
		{"OPTIONS", "/api/profiles", 405},
		{"HEAD", "/api/profiles", 405},
		{"POST", "/api/proxies", 405},
	} {
		t.Run(test.method+test.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
			if w.Code != test.status {
				t.Errorf("status=%d，預期 %d", w.Code, test.status)
			}
		})
	}
}

func TestEmbeddedUIAndSpecification(t *testing.T) {
	original := bytes.Clone(specification)
	handler, _ := testHandler(t, func(http.ResponseWriter, *http.Request) {
		t.Error("文件與 UI 不應呼叫上游")
	})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "SwaggerUIBundle") || !strings.Contains(w.Body.String(), `persistAuthorization: false`) || !strings.Contains(w.Body.String(), `validatorUrl: null`) {
		t.Fatalf("Swagger UI 或設定不符：%d %s", w.Code, w.Body.String())
	}
	assets := regexp.MustCompile(`(?:src|href)="([^"]+\.(?:js|css))"`).FindAllStringSubmatch(w.Body.String(), -1)
	if len(assets) < 3 {
		t.Fatalf("找不到完整內嵌資源：%v", assets)
	}
	for _, asset := range assets {
		if strings.HasPrefix(asset[1], "http") || strings.HasPrefix(asset[1], "//") {
			t.Fatalf("不應依賴外部資源：%s", asset[1])
		}
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/"+strings.TrimPrefix(asset[1], "/"), nil))
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Errorf("內嵌資源無法使用：%s status=%d", asset[1], w.Code)
		}
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/openapi.json", nil))
	var doc openapi3.T
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(doc.Servers) != 1 || doc.Servers[0].URL != "/api" || doc.Info.Version != "2026-11-06" {
		t.Fatalf("本機規格錯誤：%d %+v", w.Code, doc.Servers)
	}
	count := 0
	for _, item := range doc.Paths.Map() {
		count += len(item.Operations())
	}
	if count != 58 || !bytes.Equal(original, specification) {
		t.Fatal("操作遺失或原始規格被修改")
	}
}

func TestUpstreamFailure(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newHandler(target)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/profiles?secret=do-not-expose", nil))
	if w.Code != 502 || strings.Contains(w.Body.String(), "do-not-expose") || strings.Contains(w.Body.String(), target.Host) {
		t.Fatalf("上游錯誤未妥善處理：%d %s", w.Code, w.Body.String())
	}
}

func TestUpstreamErrorResponse(t *testing.T) {
	handler, _ := testHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"success":false}`)
	})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/profiles", nil))
	if w.Code != 429 || w.Header().Get("Retry-After") != "60" || w.Body.String() != `{"success":false}` {
		t.Fatalf("上游錯誤回應被改寫：%d %s", w.Code, w.Body.String())
	}
}
