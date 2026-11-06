# Control D OpenAPI 本機測試工具

以 Go 讀取既有 `openapi.yaml`，自動建立本機代理 API，並提供 Swagger UI。這是非官方工具；OpenAPI 為 2026-11-06 的文件整合快照，尚未使用實際帳號驗證。

## 架構與範圍

- `kin-openapi` 解析規格，以 HTTP 方法與路徑登錄全部 58 個操作；不手寫各操作的業務邏輯，也不產生 Go SDK。
- Go 標準庫 `httputil.ReverseProxy` 固定轉送至 `https://api.controld.com`，不提供任意目的地代理。
- `swgui/v5emb` 內嵌 Swagger UI；規格也透過 Go embed 包入執行檔。不需要 Node、Docker、CDN 或外部 YAML。
- 原始規格維持不變，`/openapi.json` 僅在記憶體將伺服器位置改成 `/api`。原始快照日期不是工具版本。
- 保留 URL 編碼、查詢參數、Authorization、X-Force-Org-Id、Content-Type、請求本文與上游回應。表單與 DELETE 本文不轉成 JSON。
- 僅依規格限制方法及路徑，不強制驗證請求或回應的資料結構，避免不完整的非官方快照阻擋真實 API。
- 規則 API 中相同路徑形狀、不同參數名稱的 GET 與 DELETE，以 HTTP 方法分別登錄。未知 API 路徑回傳 404，不支援的方法回傳 405。

```text
Swagger UI 或其他程式
    → http://127.0.0.1:8080/api/profiles
    → Go 伺服器
    → https://api.controld.com/profiles
```

## 建置與執行

開發需要 Go 1.25 以上與 PowerShell；執行已編譯的 Windows 執行檔則不需要安裝 Go。首次建置需要網路下載 Go 模組。

```powershell
.\build.ps1
.\ctrld-openapi-doc.exe
```

也可直接使用 Go：

```powershell
go build -trimpath -o ctrld-openapi-doc.exe .
```

預設只監聽 `127.0.0.1:8080`，不開放區域網路。可指定其他連接埠，或查詢工具版本：

```powershell
.\ctrld-openapi-doc.exe -port 8081
.\ctrld-openapi-doc.exe -version
```

| 路徑 | 用途 |
| --- | --- |
| `/` | Swagger UI |
| `/openapi.json` | 指向本機 `/api` 的 OpenAPI 規格 |
| `/api/...` | 規格自動建立的代理 API |

可單獨攜帶執行檔到其他 Windows amd64 電腦使用；修改規格後須重新編譯。離線可查看 UI 與規格，實際呼叫 Control D 仍需要網路。

## Token 與正式環境風險

1. 在 Control D 官方 Dashboard 建立 API Token，建議先用 Read token。
2. 開啟 `http://127.0.0.1:8080/`，在 **Authorize** 輸入 Token，不需自行加上 `Bearer`。
3. 展開操作，使用 **Try it out** 與 **Execute**。組織操作可依規格填入 `X-Force-Org-Id`。

**全部操作均連到 Control D 正式環境。新增、修改與刪除可能直接影響 DNS 設定、組織與其他正式資料；需要異動時才使用 Write token。** 不提供沙盒或模擬業務回應。

Token 僅隨請求轉送，不寫入執行檔、檔案或日誌；Swagger 不啟用授權持久儲存。請勿在不受信任的電腦使用，並於結束後關閉程式與瀏覽器頁籤。本機 API 不另設登入，只應供本機可信任程式使用。

## 版本規則

工具版號保存於 `VERSION`，格式為 `v{四碼年}.{天序}.{手動升版次數}`。

- 以手動升版當下的臺灣時間決定年與天序。
- 天序從 1 月 1 日的 `1` 開始，不補零；閏年最多為 `366`。
- 每日首次升版為 `1`；同日再次手動升版加一；跨日或跨年後的首次升版重設為 `1`。
- 一般建置直接沿用 `VERSION`，重新編譯不改版號。

```powershell
# 手動升版後建置；會修改 VERSION，應檢查並提交版本及對應變更紀錄。
.\build.ps1 -BumpVersion

# 可指定輸出位置；目的資料夾須已存在。
.\build.ps1 -Output .\ctrld-test.exe
```

例如 `2026/11/06` 的首次升版為 `v2026.310.1`，同日下一版為 `v2026.310.2`；`2026/11/07` 首次升版為 `v2026.311.1`。若升版後建置失敗，`VERSION` 仍保留更新結果，修正後直接重新建置即可，不必再次升版。此規則不覆寫原始 OpenAPI 的快照資訊。

## 開發驗證

```powershell
gofmt -w main.go main_test.go
go vet ./...
go test ./...
.\build.ps1
```

測試使用 `httptest` 模擬上游，涵蓋所有規格路由、表單 POST／PUT／DELETE、URL 編碼、查詢參數、標頭、狀態碼、回應本文、錯誤處理及內嵌 UI。不會使用真實 Token 或對正式環境自動執行寫入測試，因此不代表已驗證 Control D 實際帳號權限與服務行為。

## 相關專案

- [Control D 官方文件](https://docs.controld.com/llms.txt)
- [kin-openapi](https://github.com/getkin/kin-openapi)
- [swgui](https://github.com/swaggest/swgui)
