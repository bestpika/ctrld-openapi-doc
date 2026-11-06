package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/swaggest/swgui"
	"github.com/swaggest/swgui/v5emb"
)

//go:embed openapi.yaml
var specification []byte

//go:embed VERSION
var version string

func main() {
	port := flag.Int("port", 8080, "本機連接埠，僅監聽 127.0.0.1")
	showVersion := flag.Bool("version", false, "顯示工具版號後結束")
	flag.Parse()
	if *showVersion {
		fmt.Println(strings.TrimSpace(version))
		return
	}
	if *port < 1 || *port > 65535 {
		log.Fatal("連接埠必須介於 1 與 65535")
	}
	upstream, err := url.Parse("https://api.controld.com")
	if err != nil {
		log.Fatal(err)
	}
	handler, err := newHandler(upstream)
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", *port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("Control D 本機測試工具 %s：http://%s/；所有異動均影響正式環境", strings.TrimSpace(version), server.Addr)
	log.Fatal(server.ListenAndServe())
}

func newHandler(upstream *url.URL) (http.Handler, error) {
	doc, err := openapi3.NewLoader().LoadFromData(specification)
	if err != nil {
		return nil, fmt.Errorf("讀取內嵌 OpenAPI：%w", err)
	}
	doc.Servers = openapi3.Servers{&openapi3.Server{URL: "/api", Description: "本機 Go 代理，轉送至 Control D 正式環境"}}
	localSpec, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("產生本機 OpenAPI：%w", err)
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(upstream)
			request.Out.URL.Path = strings.TrimPrefix(request.In.URL.Path, "/api")
			request.Out.URL.RawPath = strings.TrimPrefix(request.In.URL.RawPath, "/api")
			request.Out.URL.RawQuery = request.In.URL.RawQuery
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			// 不記錄可能含憑證或個資的上游錯誤與請求。
			http.Error(w, "無法連線至 Control D 上游服務", http.StatusBadGateway)
		},
	}
	apiMux := http.NewServeMux()
	for path, item := range doc.Paths.Map() {
		operations := item.Operations()
		allowed := make([]string, 0, len(operations))
		for method := range operations {
			allowed = append(allowed, method)
		}
		slices.Sort(allowed)
		for method := range operations {
			apiMux.HandleFunc(method+" /api"+path, func(w http.ResponseWriter, r *http.Request) {
				// ServeMux 的 GET 預設也匹配 HEAD，但代理僅開放規格列出的操作。
				if r.Method != method {
					w.Header().Set("Allow", strings.Join(allowed, ", "))
					http.Error(w, "此 API 不支援這個 HTTP 方法", http.StatusMethodNotAllowed)
					return
				}
				proxy.ServeHTTP(w, r)
			})
		}
	}
	// API 獨立比對，保留 ServeMux 的 404 與 405，不落入 Swagger UI。
	mux := http.NewServeMux()
	mux.Handle("/api/", apiMux)
	mux.Handle("/api", apiMux)
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(localSpec)
	})
	mux.Handle("/", v5emb.NewHandlerWithConfig(swgui.Config{
		Title:       "Control D 本機 API 測試",
		SwaggerJSON: "/openapi.json",
		BasePath:    "/",
		SettingsUI: map[string]string{
			"persistAuthorization": "false",
			"validatorUrl":         "null",
		},
	}))
	return mux, nil
}
