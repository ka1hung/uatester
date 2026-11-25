**繁體中文** | [English](uaexpert-test.en.md)

# 用 UaExpert 對照驗證 opcua 套件

目的：同一台 **uatester 伺服器**（底層是 github.com/ka1hung/opcua），用兩個用戶端各做一次同樣的操作，比較結果：

- **UaExpert**（Unified Automation，第三方、業界常用）：當作基準。它做得到、結果正確 → 我們的**伺服器**沒問題。
- **uatester 用戶端**（右半邊，也是 opcua 套件）：結果要和 UaExpert 一致。不一致 → 我們的**用戶端**有問題。

這份文件寫給在 Claude 桌面 App 裡用 Computer use 操作的 Claude，也給在旁邊看的人。每一步寫了兩邊各要做什麼、應該看到什麼；結果照最後一節的格式回報。

## 安全規則

- **只連本文件列出的測試伺服器**（本機 4841 埠、192.168.3.127:4841）。
- **不要連、也不要寫入** `192.168.3.205:46010`（公司現場的舊版 MBController）以及任何 4840 埠的 MBController。
- 測試帳號只用 uatester 內建的 `admin/admin`、`viewer/viewer`。

## 準備

| 項目 | 內容 |
|---|---|
| UaExpert | 已安裝在這台 Windows（免費版即可） |
| uatester 桌面版 | `C:\Users\kaihung\Desktop\ccc\uatester\bin\uatester.exe`：左半邊是伺服器、右半邊是用戶端，下方是兩邊共用的記錄；畫面太擠時，上方的「只看伺服器」「只看用戶端」可只留一邊，「並排」再放回來 |
| 帳號 | `admin` / `admin`：可讀寫、可確認警報、可呼叫方法；`viewer` / `viewer`：唯讀；匿名：唯讀 |
| 測試節點 | `ns=2;s=P0000_Temp` … `P0099_Temp`（Double，每 0.5 秒變化）、`ns=2;s=Setpoint`（Int16）、`ns=2;s=Running`（Boolean）、方法 `ns=2;s=Plant.Reset`（輸入 Reason 字串，輸出 Done 布林） |
| 警報 | 每第 10 個點位（P0000、P0010、P0020…）超過 80 自動觸發 `High` 警報；也可在 uatester 左半邊手動觸發 |

## 測試一：本機伺服器（Windows 64 位元）

### 1-0. 啟動伺服器

1. 開 `uatester.exe`。
2. 左半邊「伺服器」：**監聽位址**改成 `0.0.0.0:4841`，**對外主機名稱 / IP** 填 `127.0.0.1`，其他不動。
3. 按「啟動伺服器」。
4. 應該看到：狀態「執行中」，下方記錄出現 7 行 `endpoint url=opc.tcp://127.0.0.1:4841 ...`。

### 1-1. 端點與匿名連線

| | 操作 | 應該看到 |
|---|---|---|
| **UaExpert** | Project 窗格 → Servers 按右鍵 → **Add...** → Custom Discovery 雙擊 **< Double click to Add Server... >**，輸入 `opc.tcp://127.0.0.1:4841` → 展開，看端點清單 → 選 **None - None**，Authentication 選 **Anonymous** → OK → 對伺服器按右鍵 **Connect**（跳憑證視窗就按 **Trust Server Certificate** → Continue） | 端點清單共 7 個：None，以及 Basic256Sha256、Aes128_Sha256_RsaOaep、Aes256_Sha256_RsaPss 各有 Sign、Sign & Encrypt；連上後 Address Space 出現 `Root`，Log 無紅字 |
| **uatester 用戶端** | 右半邊「端點 URL」填 `opc.tcp://127.0.0.1:4841` → 按「查詢端點」→ 再按「連線」（加密選 None，帳號空白＝匿名） | 端點清單與 UaExpert 相同（7 個）；連線後狀態「已連線」，下面一行顯示伺服器 uatester、狀態 Running |

### 1-2. 瀏覽與節點屬性

| | 操作 | 應該看到 |
|---|---|---|
| **UaExpert** | Address Space 展開 **Objects → Plant**，點 `P0000_Temp` 看 Attributes 窗格 | 子節點：P0000_Temp…P0099_Temp、Setpoint、Running、Reset；`P0000_Temp` DataType Double、Description simulated temperature |
| **uatester 用戶端** | 「瀏覽」分頁，樹狀展開 **Objects → Plant**，點 `P0000_Temp` | 子節點與 UaExpert 相同；右側顯示說明 simulated temperature、資料型別 Double、權限「可讀、有歷史」（匿名不可寫） |

### 1-3. 讀值與即時更新

| | 操作 | 應該看到 |
|---|---|---|
| **UaExpert** | 把 `P0000_Temp`、`P0010_Temp`、`Setpoint`、`Running` 拖進 **Data Access View** | 溫度約每 0.5 秒變化、Status Good；Setpoint = 22、Running = true |
| **uatester 用戶端** | 「監看」分頁，依序輸入 `ns=2;s=P0000_Temp`、`ns=2;s=P0010_Temp`、`ns=2;s=Setpoint`、`ns=2;s=Running` 各按「加入」 | 數值和 UaExpert 同時間的值一致、同步變化；Status Good |

### 1-4. 匿名不能寫

| | 操作 | 應該看到 |
|---|---|---|
| **UaExpert** | Data Access View 對 `Setpoint` 的 Value 雙擊，輸入 `30`，Enter | 失敗，Log 出現 `BadUserAccessDenied`，值仍 22 |
| **uatester 用戶端** | 「瀏覽」分頁點 `Setpoint`，寫入框輸入 `30`，按「寫入」 | 失敗，紅色錯誤訊息含 `BadUserAccessDenied`，值仍 22 |

### 1-5. 用 admin 寫入

兩邊先斷線，改用帳號登入：

| | 操作 | 應該看到 |
|---|---|---|
| **UaExpert** | 伺服器按右鍵 **Disconnect** → **Properties...**，Authentication 改 **Username** `admin` / `admin` → OK → **Connect**；Data Access View 把 `Setpoint` 改 `30`、`Running` 改 `false` | 兩個寫入成功；uatester 記錄出現 `client write user=admin node=ns=2;s=Setpoint value=30` |
| **uatester 用戶端** | 按「中斷」，帳號 `admin`、密碼 `admin`，按「連線」；「瀏覽」分頁點 `Setpoint` 寫入 `31`，點 `Running` 寫入 `true` | 兩個寫入成功（顯示「已寫入」）；UaExpert 的 Data Access View **同步**看到 31 和 true |
| **兩邊** | 把 `Setpoint` 寫成 `40000` | 兩邊都失敗（超出 Int16 範圍：`BadOutOfRange` 或 `BadTypeMismatch`），錯誤碼相同 |

### 1-6. 呼叫方法

| | 操作 | 應該看到 |
|---|---|---|
| **UaExpert** | Objects → Plant → **Reset** 按右鍵 → **Call...**，Reason 填 `from uaexpert`，按 **Call** | 結果 Good，Done = true；uatester 記錄出現 `method Reset user=admin reason=from uaexpert` |
| **uatester 用戶端** | 「瀏覽」分頁點 **Reset**，參數 Reason 填 `from uatester`，按「呼叫」 | 顯示「呼叫成功」，Done = true；記錄出現 `reason=from uatester` |

### 1-7. 事件

兩邊先開好接收，再觸發事件：

| | 操作 | 應該看到 |
|---|---|---|
| **UaExpert** | **Document → Add...** 選 **Event View**；把 **Objects → Server** 拖進 Configuration 區 | — |
| **uatester 用戶端** | 「警報與事件」分頁，通知者保持 `i=2253`，按「訂閱」 | — |
| **觸發** | uatester 左半邊「警報與事件」分頁，「事件訊息」填 `compare event`，按「送出事件」 | **兩邊都**收到 `compare event`，時間、來源（Plant）、嚴重度（300）一致；也會陸續收到 `P00x0_Temp high (> 80)` 等警報事件 |

### 1-8. 警報（Alarms & Conditions）

| | 操作 | 應該看到 |
|---|---|---|
| **UaExpert** | **Document → Add...** 選 **Alarms & Conditions**；把 **Objects → Server** 拖進去 | 列出作用中的 `High` 警報（Severity 700），連線前就作用的也在 |
| **uatester 用戶端** | 「警報與事件」分頁已訂閱（1-7）；按「重新取得目前警報」 | 「目前警報」列表與 UaExpert 相同（同樣的點位、狀態、嚴重度） |
| **觸發** | uatester 左半邊：點位 `P0005_Temp`、嚴重度 `900`、訊息 `compare alarm`，按「觸發警報」 | **兩邊立刻**出現這個警報，Severity 900 |
| **UaExpert 確認** | 在 UaExpert 對 `compare alarm` 按右鍵 → **Acknowledge**，Comment `ok` | UaExpert 顯示 Acknowledged；**uatester 用戶端**的「目前警報」同步變「已確認」；左半邊伺服器的警報也顯示已確認 |
| **uatester 確認** | 再觸發一次：點位 `P0006_Temp`、訊息 `compare alarm 2`；這次在 **uatester 用戶端**「目前警報」按該列的「確認」 | uatester 顯示已確認；**UaExpert** 同步變 Acknowledged |

### 1-9. 歷史

先讓伺服器跑至少 2 分鐘：

| | 操作 | 應該看到 |
|---|---|---|
| **UaExpert** | **Document → Add...** 選 **History Trend View**；拖入 `P0001_Temp`，時間範圍最近 5 分鐘，按 **Update** | 一條起伏的曲線 |
| **uatester 用戶端** | 「歷史」分頁，節點 `ns=2;s=P0001_Temp`、方式「原始值」、最近 `5` 分鐘，按「查詢」 | 曲線形狀和 UaExpert 一致；資料表筆數與時間範圍相符 |
| **uatester 聚合** | 方式改「平均」、區間 `60` 秒，查詢 | 每分鐘一筆平均值（UaExpert 免費版可能無法做聚合，這項只看 uatester） |

### 1-10. 加密連線與憑證信任

| | 操作 | 應該看到 |
|---|---|---|
| **UaExpert** | Add 伺服器 `opc.tcp://127.0.0.1:4841`，選 **Basic256Sha256 - Sign & Encrypt**，Username `admin` / `admin`，Connect（伺服器憑證按 Trust） | 第一次**被拒**（`BadSecurityChecksFailed` 或 `BadCertificateUntrusted` 一類）——伺服器還沒信任 UaExpert，這是正確的 |
| **uatester 用戶端** | 中斷後，加密選 `Basic256Sha256:SignAndEncrypt`，帳號 admin，連線 | 第一次同樣**被拒** |
| **信任** | uatester 左半邊「憑證」分頁按「重新整理」，兩張憑證（UaExpert、uatester）各按「信任」 | — |
| **再連** | 兩邊各再連一次，讀 `Setpoint` | 兩邊都連上、讀到相同的值 |
| **另一種原則** | 兩邊都改用 **Aes256_Sha256_RsaPss - Sign & Encrypt** 再連 | 兩邊都成功 |

## 測試二：armv7 伺服器（192.168.3.127，32 位元）

驗證 32 位元平台（現場工控機）的修正。**伺服器要由使用者在 .127 上啟動**（Computer use 不操作 SSH）：

```
chmod +x uatest-linux-armv7
./uatest-linux-armv7 server -addr 0.0.0.0:4841 -host 192.168.3.127 -points 100
```

確認伺服器在跑之後，兩邊的端點 URL 都改用 `opc.tcp://192.168.3.127:4841`（用 IP，內網沒有 DNS），依序做 **1-1 到 1-7、1-10**。差異：

- .127 上沒有 uatester 的視窗：1-7 的「送出事件」改成兩邊各呼叫一次 Reset（方法會發事件）；1-8 只比對自動觸發的 `High` 警報與確認。
- 1-10 的「信任」：使用者把 .127 上 `pki/rejected/certs/` 裡的檔案移到 `pki/trusted/certs/`。
- 特別注意 1-1：舊版套件在 32 位元上一連線就斷（`BadTCPInternalError`），現在應該正常。

## 回報格式

每一步填一列，全部做完貼回開發的對話：

| 步驟 | UaExpert | uatester 用戶端 | 兩邊一致？ | 備註（失敗或不一致時必填） |
|---|---|---|---|---|
| 1-1 | 通過 / 失敗 | 通過 / 失敗 | 是 / 否 | |
| 1-2 | | | | |
| … | | | | |

失敗或不一致時另外記下：
- UaExpert **Log** 視窗中該步驟前後的紅色訊息（整行複製）。
- uatester 下方記錄中同一時間的行，以及畫面上的錯誤訊息。
- 兩邊當下的截圖。
