**繁體中文** | [English](uaexpert-test.en.md)

# 用 UaExpert 驗證 opcua 套件的伺服器

目的：用第三方用戶端 **UaExpert**（Unified Automation）連 **uatester 的伺服器**（底層是 github.com/ka1hung/opcua），確認我們的 OPC UA 伺服器和業界工具互通。用戶端那一側已另外用 uatest 驗證過，這份只測伺服器。

這份文件寫給在 Claude 桌面 App 裡用 Computer use 操作 UaExpert 的 Claude，也給在旁邊看的人。每一步寫了「做什麼」和「應該看到什麼」；結果照最後一節的格式回報。

## 安全規則

- **只連本文件列出的測試伺服器**（本機 4841 埠、192.168.3.127:4841）。
- **不要連、也不要寫入** `192.168.3.205:46010`（公司現場的舊版 MBController）以及任何 4840 埠的 MBController。
- 測試帳號只用 uatester 內建的 `admin/admin`、`viewer/viewer`。

## 準備

| 項目 | 內容 |
|---|---|
| UaExpert | 已安裝在這台 Windows（免費版即可） |
| uatester 桌面版 | `C:\Users\kaihung\Desktop\ccc\uatester\bin\uatester.exe` |
| 帳號 | `admin` / `admin`：可讀寫、可確認警報、可呼叫方法；`viewer` / `viewer`：唯讀；匿名：唯讀 |
| 測試節點 | `ns=2;s=P0000_Temp` … `P0099_Temp`（Double，每 0.5 秒變化）、`ns=2;s=Setpoint`（Int16）、`ns=2;s=Running`（Boolean）、方法 `ns=2;s=Plant.Reset`（輸入 Reason 字串，輸出 Done 布林） |
| 警報 | 每第 10 個點位（P0000、P0010、P0020…）超過 80 自動觸發 `High` 警報；也可在 uatester 手動觸發 |

## 測試一：本機伺服器（Windows 64 位元）

### 1-0. 啟動伺服器（uatester）

1. 開 `uatester.exe`。
2. 左半邊「伺服器」：**監聽位址**改成 `0.0.0.0:4841`，**對外主機名稱 / IP** 填 `127.0.0.1`，其他不動。
3. 按「啟動伺服器」。
4. 應該看到：狀態變「執行中」，下方記錄出現 7 行 `endpoint url=opc.tcp://127.0.0.1:4841 ...`。

### 1-1. 新增伺服器並匿名連線

1. UaExpert 左邊 **Project** 窗格 → **Servers** 按右鍵 → **Add...**。
2. **Custom Discovery** 底下雙擊 **< Double click to Add Server... >**，輸入 `opc.tcp://127.0.0.1:4841`，OK。
3. 展開剛加入的伺服器，選 **None - None (uatcp-uasc-uabinary)** 端點。
4. 下方 **Authentication Settings** 選 **Anonymous**，按 **OK**。
5. 在 Project 窗格對這個伺服器按右鍵 → **Connect**。如果跳出憑證視窗，按 **Trust Server Certificate** 再按 **Continue**。
6. 應該看到：左下 **Address Space** 窗格出現 `Root`；下方 **Log** 視窗沒有紅色錯誤。

### 1-2. 瀏覽

1. Address Space 展開 **Objects → Plant**。
2. 應該看到：`P0000_Temp` … `P0099_Temp`、`Setpoint`、`Running`、`Reset`。
3. 點 `P0000_Temp`，右邊 **Attributes** 窗格：DataType = Double，Description = simulated temperature。

### 1-3. 讀值與即時更新

1. 把 `P0000_Temp`、`P0010_Temp`、`Setpoint`、`Running` 拖進中間的 **Data Access View**。
2. 應該看到：兩個溫度的 Value 每 0.5 秒左右變一次，Status 為 Good；`Setpoint` = 22，`Running` = true。

### 1-4. 匿名不能寫

1. 在 Data Access View 對 `Setpoint` 的 **Value** 欄雙擊，輸入 `30`，按 Enter。
2. 應該看到：寫入失敗，Log 視窗出現 `BadUserAccessDenied`；值仍是 22。

### 1-5. 用 admin 寫入

1. Project 窗格對伺服器按右鍵 → **Disconnect**。
2. 再按右鍵 → **Properties...**，Authentication Settings 改 **Username**，填 `admin` / `admin`，OK。
3. 按右鍵 → **Connect**。
4. 把 `Setpoint`、`Running` 重新拖進 Data Access View（若已不在）。
5. `Setpoint` 改成 `30`，`Running` 改成 `false`（Boolean 欄位用下拉或輸入 false）。
6. 應該看到：兩個寫入都成功、值變更；uatester 下方記錄出現 `client write user=admin node=ns=2;s=Setpoint value=30`。
7. 再把 `Setpoint` 改成 `40000`。應該看到：失敗（超出 Int16 範圍，`BadOutOfRange` 或 `BadTypeMismatch`）。

### 1-6. 呼叫方法

1. Address Space：Objects → Plant → **Reset** 按右鍵 → **Call...**。
2. Reason 填 `uaexpert test`，按 **Call**。
3. 應該看到：結果 Good，輸出 Done = true；uatester 記錄出現 `method Reset user=admin reason=uaexpert test`。

### 1-7. 事件

1. 選單 **Document → Add...**，類型選 **Event View**，OK。
2. 把 Address Space 的 **Objects → Server** 拖進 Event View 上方的 **Configuration** 區。
3. 重做 1-6 呼叫一次 Reset。
4. 應該看到：Events 列表出現 `reset by admin: ...` 的事件；另外會陸續出現 `P00x0_Temp high (> 80)` / `back to normal` 的警報事件（有些點要等幾十秒才會超過 80）。

### 1-8. 警報（Alarms & Conditions）

1. **Document → Add...**，類型選 **Alarms & Conditions** 視窗，OK。
2. 把 **Objects → Server** 拖進去（或在該視窗按 Refresh 圖示）。
3. 應該看到：作用中的 `High` 警報（Severity 700），連線前就已作用的也會出現（ConditionRefresh）。
4. 回 uatester：左半邊「警報與事件」分頁，點位填 `P0005_Temp`、嚴重度 `900`、訊息 `uaexpert manual`，按「觸發警報」。
5. 應該看到：UaExpert Alarms 視窗**立刻**出現這個警報（Severity 900）。
6. 在 UaExpert 對這個警報按右鍵 → **Acknowledge**，Comment 填 `ok`，確認。
7. 應該看到：UaExpert 中該警報的 AckedState 變 Acknowledged；uatester「警報與事件」分頁該列顯示「已確認」，記錄出現 `client acknowledge user=admin`。

### 1-9. 歷史

1. 先讓伺服器跑至少 2 分鐘。
2. **Document → Add...**，類型選 **History Trend View**，OK。
3. 把 `P0001_Temp` 拖進去，時間範圍設最近 5 分鐘，按 **Update**。
4. 應該看到：一條上下起伏的曲線。

### 1-10. 加密連線與憑證信任

1. Project 窗格 → Servers 按右鍵 → Add...，同樣用 `opc.tcp://127.0.0.1:4841`，這次選 **Basic256Sha256 - Sign & Encrypt** 端點，Username `admin` / `admin`。
2. Connect。UaExpert 跳伺服器憑證視窗 → **Trust Server Certificate** → Continue。
3. 應該看到：第一次**被拒**（Log 出現 `BadSecurityChecksFailed` 或 `BadCertificateUntrusted` 一類）——伺服器還沒信任 UaExpert 的憑證，這是正確行為。
4. 回 uatester：左半邊「憑證」分頁，列表出現 UaExpert 的憑證，按「信任」。
5. UaExpert 再 Connect 一次。應該看到：成功連上，可以讀值、寫入。
6. 同樣試一次 **Aes256_Sha256_RsaPss - Sign & Encrypt**。

## 測試二：armv7 伺服器（192.168.3.127，32 位元）

這一段驗證 32 位元平台（現場工控機）的修正。**伺服器要由使用者在 .127 上啟動**（Computer use 不操作 SSH）：

```
chmod +x uatest-linux-armv7
./uatest-linux-armv7 server -addr 0.0.0.0:4841 -host 192.168.3.127 -points 100
```

確認伺服器在跑之後：

1. UaExpert 新增伺服器 `opc.tcp://192.168.3.127:4841`（不要用電腦名稱，內網沒有 DNS）。
2. 依序做測試一的 **1-1 到 1-8** 與 **1-10**（1-10 的「信任」改為：使用者把 .127 上 `pki/rejected/certs/` 裡的檔案移到 `pki/trusted/certs/`）。
3. 特別注意 1-1：舊版套件在 32 位元上會在連線時斷開（`BadTCPInternalError`），現在應該正常連上。

## 回報格式

每一項填一列，全部做完貼回給開發的對話：

| 項目 | 結果 | 備註（失敗時必填） |
|---|---|---|
| 1-1 | 通過 / 失敗 | Log 視窗的完整錯誤訊息、當時的畫面截圖 |
| 1-2 | | |
| … | | |

失敗時另外記下：
- UaExpert **Log** 視窗中該步驟前後的紅色訊息（整行複製）。
- uatester 下方記錄中同一時間的行。
- 失敗那一刻的截圖。
