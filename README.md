**繁體中文** | [English](README.en.md)

# uatester

OPC UA 測試工具：一個模擬伺服器加一個用戶端，可以互相測試，也可以拿來測其他 OPC UA 產品（UaExpert、SCADA、PLC）。底層使用 [ka1hung/opcua](https://github.com/ka1hung/opcua)。

有兩種版本，共用同一份核心程式（`core/`）：

| 版本 | 執行檔 | 用在哪裡 |
|---|---|---|
| 桌面版 | `uatester.exe`（Wails v3） | Windows 筆電：一個視窗裡同時操作伺服器與用戶端 |
| 命令列版 | `uatest` | 沒有螢幕的工控機（Linux armv7）長時間測試、現場用指令快速診斷 |

## 下載

到 [Releases](https://github.com/ka1hung/uatester/releases) 下載編譯好的檔案：

- 桌面版：Windows（`.zip`）、macOS（Intel 與 Apple Silicon 通用的 `.app`）、Linux amd64。
- 命令列版 `uatest`：Windows、macOS、Linux amd64 / arm64 / armv7，單一執行檔、不需要安裝任何東西。

macOS 版沒有經過 Apple 簽章公證，第一次開啟請對 `uatester.app` 按右鍵 →「打開」。Linux 桌面版需要 `libgtk-3-0` 與 `libwebkit2gtk-4.1-0`。

## 桌面版

左半邊是**伺服器**，右半邊是**用戶端**，下方是兩邊共用的記錄。

**伺服器**
- 設定監聽位址、點位數、更新間隔、歷史筆數後按「啟動伺服器」。
- 點位：數值會自動模擬變化；可以手動設定任一點位的值（之後模擬不再改它，按「恢復模擬」才回來），也可以整個暫停模擬。
- 警報與事件：每第 10 個點位超過 80 會自動觸發 High 警報；也可以手動觸發 / 解除警報、確認警報、送出事件。
- 連線中的用戶端：誰連進來、用哪個帳號、幾個訂閱。
- 憑證：用加密方式連線但還沒被信任的用戶端會列在這裡，按「信任」即可。
- 帳號：`admin/admin` 可讀寫、可確認警報；`viewer/viewer` 與匿名只能讀。

**用戶端**
用戶端的功能和伺服器一一對應，可以拿來測本工具的伺服器，也可以測其他廠牌的伺服器。

- 連線：輸入端點 URL、選加密方式與帳號。按「查詢端點」會列出伺服器提供的加密組合與登入方式，點一下就帶入；在伺服器的「端點」清單按「用戶端連這裡」也可以。連線後顯示伺服器的產品、版本與狀態。
- 瀏覽：節點樹。選到節點會顯示說明、資料型別與這個帳號的權限（可讀、可寫、有歷史、可執行）。
  - 變數：自動讀值，可以寫入、加入監看、查歷史。
  - 方法：依方法定義列出參數欄位，填好按「呼叫」，顯示輸出結果。
  - 有事件的物件：可以直接訂閱它的事件、查它的事件歷史。
- 監看：訂閱多個節點，數值即時更新；可以設定 deadband（絕對值或百分比）。
- 警報與事件：訂閱 Server 物件或某個區域的事件。「目前警報」列出作用中與還沒確認的警報，可以直接確認；連線前就已作用的警報也會補上。「事件紀錄」列出最近收到的事件。
- 歷史：原始值、聚合值（平均、最小、最大、筆數、區間第一 / 最後一筆）以折線圖與資料表顯示；也可以查事件歷史。
- 壓力測試：多個用戶端同時訂閱本工具伺服器的點位，量測每秒通知數。

伺服器與用戶端的憑證存在 `%APPDATA%\uatester`，換位置執行也會沿用，其他產品只需要信任一次。

介面可以切換繁體中文 / English，顏色跟著 Windows 的淺色 / 深色設定。

## 命令列版

```
uatest server    [-addr 0.0.0.0:4840 -host 192.168.1.10 -points 3000 -interval 500ms -history 200 -stats 1m]
uatest endpoints -ep opc.tcp://host:4840
uatest status    -ep ...
uatest browse    -ep ... [-node i=85]
uatest attrs     -ep ... -node "ns=2;s=P0000_Temp"
uatest read      -ep ... -node "ns=2;s=P0000_Temp" [-node ...]
uatest write     -ep ... -node "ns=2;s=Setpoint" -value 23 -user admin -pass admin
uatest subscribe -ep ... -node "ns=2;s=P0000_Temp" [-interval 500ms -deadband 0.5 | -pctdeadband 1]
uatest call      -ep ... -object "ns=2;s=Plant" -method "ns=2;s=Plant.Reset" -arg "reason" -user admin -pass admin
uatest events    -ep ... [-node "ns=2;s=Plant"] [-ack -comment "seen"] -user admin -pass admin
uatest history   -ep ... -node "ns=2;s=P0000_Temp" [-agg average -from 1h -interval 10m]
uatest history   -ep ... -node "ns=2;s=Plant" -agg events -from 1h
uatest stress    -ep ... -clients 10 -points 3000 -interval 500ms -duration 5m
```

- 加密：`-security none`（預設）、`auto`（至少簽章，不會退回 None），或 `原則:模式`，例如 `Basic256Sha256:SignAndEncrypt`。
- `-pki 資料夾` 會保存本工具的用戶端憑證，伺服器只要信任一次。
- `uatest <指令> -h` 列出該指令的所有參數。

**armv7 長時間測試**（在工控機上）：

```
uatest server -addr 0.0.0.0:4840 -host <IP> -points 3000 -interval 500ms -history 0 -stats 10m > soak.log 2>&1
# 另一台機器
uatest stress -ep opc.tcp://<IP>:4840 -clients 10 -points 3000 -interval 500ms -duration 72h
```

判讀：`soak.log` 裡的 `goroutines`、`heapMB`、`sysMB` 在 72 小時內不應持續上升。

## 編譯

需要 Go 1.26 以上。桌面版需要 Windows 與 WebView2（Windows 10 / 11 已內建），不需要 Node.js 或 C 編譯器。以下指令用 Git Bash；在 PowerShell 裡，環境變數改用 `$env:GOOS="linux"` 這種寫法。

```
# 桌面版（Windows）
go build -tags production -trimpath -ldflags "-H windowsgui -s -w" -o bin/uatester.exe ./app

# 命令列版
go build -o bin/uatest.exe ./cmd/uatest
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o bin/uatest-armv7 ./cmd/uatest
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/uatest-linux ./cmd/uatest
```

開發時可以用瀏覽器預覽桌面版介面（功能完整，只是改成在瀏覽器裡跑）：

```
go build -tags server -o bin/uatester-web.exe ./app
WAILS_SERVER_PORT=8080 bin/uatester-web.exe      # 然後開 http://localhost:8080
```

## 發布新版本

推一個 `v` 開頭的 tag，GitHub Actions 就會編譯所有平台並建立 Release：

```
git tag v0.2.0
git push origin v0.2.0
```

版本號會從 tag 帶入程式（視窗標題、`uatest version`）。在 GitHub 的 Actions 頁面手動執行 **release** workflow，只會編譯、不會發布，可以用來先確認能不能編譯。

## 測試

```
go test ./...
```

`core` 的測試會啟動模擬伺服器，用用戶端實際連線：讀寫權限、手動設值、歷史、伺服器觸發警報 → 用戶端收到並確認、方法呼叫、事件、憑證信任、壓力測試。

## 目錄

| 路徑 | 內容 |
|---|---|
| `core/` | 共用核心：模擬伺服器、記憶體歷史、憑證管理、用戶端連線、歷史讀取、壓力測試 |
| `cmd/uatest/` | 命令列版 |
| `app/` | 桌面版：Go 服務（`*_service.go`）與介面（`frontend/`，純 HTML / CSS / JS，不需要建置） |

## 相依

使用 [`github.com/ka1hung/opcua`](https://github.com/ka1hung/opcua)。開發 opcua 時若要直接用旁邊的 checkout，可暫時加上 `replace github.com/ka1hung/opcua => ../opcua`，提交前記得拿掉。

## 授權

MIT。
