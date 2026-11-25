[繁體中文](uaexpert-test.md) | **English**

# Cross-checking the opcua package against UaExpert

Goal: on one **uatester server** (built on github.com/ka1hung/opcua), do the same operations with two clients and compare:

- **UaExpert** (Unified Automation, third party, widely used): the reference. If it works and the results are right, our **server** is fine.
- **uatester's client** (right half, also the opcua package): must match UaExpert. A difference means our **client** is wrong.

Written for Claude operating through Computer use in the Claude desktop app, and for the person watching. Each step says what to do on each side and what should happen; report in the format of the last section.

## Safety rules

- **Connect only to the test servers listed here** (local port 4841, 192.168.3.127:4841).
- **Do not connect to or write to** `192.168.3.205:46010` (the old MBController on site) or any MBController on port 4840.
- Use only uatester's built-in accounts `admin/admin` and `viewer/viewer`.

## Setup

| Item | Details |
|---|---|
| UaExpert | Installed on this Windows machine (the free edition is enough) |
| uatester desktop | `C:\Users\kaihung\Desktop\ccc\uatester\bin\uatester.exe`: server on the left, client on the right, shared log at the bottom; when it gets crowded, **只看伺服器** (Server only) or **只看用戶端** (Client only) at the top keeps one side, **並排** (Side by side) brings both back |
| Accounts | `admin` / `admin`: read, write, acknowledge alarms, call methods; `viewer` / `viewer`: read only; anonymous: read only |
| Test nodes | `ns=2;s=P0000_Temp` … `P0099_Temp` (Double, changing every 0.5 s), `ns=2;s=Setpoint` (Int16), `ns=2;s=Running` (Boolean), method `ns=2;s=Plant.Reset` (input Reason string, output Done Boolean) |
| Alarms | Every tenth point (P0000, P0010, P0020…) raises a `High` alarm above 80; alarms can also be raised by hand on uatester's left half |

uatester's interface is in Chinese by default; the "English" button at the top right switches it. The labels below give the Chinese text with the English in brackets.

## Test 1: local server (Windows, 64-bit)

### 1-0. Start the server

1. Open `uatester.exe`.
2. Left half (Server): set **監聽位址** (Listen address) to `0.0.0.0:4841` and **對外主機名稱 / IP** (Host name / IP for clients) to `127.0.0.1`.
3. Press **啟動伺服器** (Start server).
4. Expect: state Running; the log shows 7 lines `endpoint url=opc.tcp://127.0.0.1:4841 ...`.

### 1-1. Endpoints and anonymous connection

| | Do | Expect |
|---|---|---|
| **UaExpert** | Project pane → right-click Servers → **Add...** → under Custom Discovery double-click **< Double click to Add Server... >**, enter `opc.tcp://127.0.0.1:4841` → expand and look at the endpoints → pick **None - None**, Authentication **Anonymous** → OK → right-click the server → **Connect** (on a certificate dialog: **Trust Server Certificate** → Continue) | 7 endpoints: None, and Basic256Sha256, Aes128_Sha256_RsaOaep, Aes256_Sha256_RsaPss each with Sign and Sign & Encrypt; after connecting, `Root` in the Address Space, no red lines in the Log |
| **uatester client** | Right half: Endpoint URL `opc.tcp://127.0.0.1:4841` → **查詢端點** (Endpoints) → **連線** (Connect) with security None and an empty user (anonymous) | The same 7 endpoints; after connecting, state Connected and a line showing server uatester, state Running |

### 1-2. Browse and node attributes

| | Do | Expect |
|---|---|---|
| **UaExpert** | Address Space: **Objects → Plant**; select `P0000_Temp` and read the Attributes pane | Children P0000_Temp…P0099_Temp, Setpoint, Running, Reset; `P0000_Temp` DataType Double, Description simulated temperature |
| **uatester client** | Tab **瀏覽** (Browse): expand **Objects → Plant**, select `P0000_Temp` | Same children; on the right description simulated temperature, data type Double, access read and history (anonymous cannot write) |

### 1-3. Read and live updates

| | Do | Expect |
|---|---|---|
| **UaExpert** | Drag `P0000_Temp`, `P0010_Temp`, `Setpoint`, `Running` into the **Data Access View** | Temperatures change about every 0.5 s, Status Good; Setpoint = 22, Running = true |
| **uatester client** | Tab **監看** (Watch): add `ns=2;s=P0000_Temp`, `ns=2;s=P0010_Temp`, `ns=2;s=Setpoint`, `ns=2;s=Running` one by one | Values match UaExpert's at the same moment and change together; Status Good |

### 1-4. Anonymous cannot write

| | Do | Expect |
|---|---|---|
| **UaExpert** | Data Access View: double-click `Setpoint`'s Value, type `30`, Enter | Fails, Log shows `BadUserAccessDenied`, value stays 22 |
| **uatester client** | Tab Browse: select `Setpoint`, type `30` in the write box, press **寫入** (Write) | Fails, red error containing `BadUserAccessDenied`, value stays 22 |

### 1-5. Write as admin

Disconnect both and log in with an account:

| | Do | Expect |
|---|---|---|
| **UaExpert** | Right-click the server → **Disconnect** → **Properties...**, Authentication **Username** `admin` / `admin` → OK → **Connect**; set `Setpoint` to `30` and `Running` to `false` | Both writes succeed; uatester's log shows `client write user=admin node=ns=2;s=Setpoint value=30` |
| **uatester client** | **中斷** (Disconnect); user `admin`, password `admin`, **連線** (Connect); Browse: write `31` to `Setpoint` and `true` to `Running` | Both succeed ("Written"); UaExpert's Data Access View shows 31 and true **at the same time** |
| **Both** | Write `40000` to `Setpoint` | Both fail (out of Int16 range: `BadOutOfRange` or `BadTypeMismatch`) with the same status |

### 1-6. Method call

| | Do | Expect |
|---|---|---|
| **UaExpert** | Objects → Plant → right-click **Reset** → **Call...**, Reason `from uaexpert`, **Call** | Result Good, Done = true; uatester's log shows `method Reset user=admin reason=from uaexpert` |
| **uatester client** | Browse: select **Reset**, Reason `from uatester`, press **呼叫** (Call) | "Call succeeded", Done = true; log shows `reason=from uatester` |

### 1-7. Events

Open both receivers first, then fire the event:

| | Do | Expect |
|---|---|---|
| **UaExpert** | **Document → Add...** → **Event View**; drag **Objects → Server** into the Configuration area | — |
| **uatester client** | Tab **警報與事件** (Alarms & events): keep notifier `i=2253`, press **訂閱** (Subscribe) | — |
| **Fire** | uatester left half, tab Alarms & events: event message `compare event`, press **送出事件** (Send event) | **Both** receive `compare event` with the same time, source (Plant) and severity (300); `P00x0_Temp high (> 80)` alarm events arrive over time too |

### 1-8. Alarms & Conditions

| | Do | Expect |
|---|---|---|
| **UaExpert** | **Document → Add...** → **Alarms & Conditions**; drag **Objects → Server** in | Active `High` alarms (Severity 700), including those active before connecting |
| **uatester client** | Already subscribed in 1-7; press **重新取得目前警報** (Get current alarms again) | Current alarms match UaExpert's (same points, states, severities) |
| **Fire** | uatester left half: point `P0005_Temp`, severity `900`, message `compare alarm`, **觸發警報** (Raise alarm) | The alarm appears **at once on both sides**, Severity 900 |
| **Ack in UaExpert** | Right-click `compare alarm` → **Acknowledge**, comment `ok` | UaExpert shows Acknowledged; **uatester's client** turns it to acknowledged at the same time; the server side shows acknowledged too |
| **Ack in uatester** | Raise another: point `P0006_Temp`, message `compare alarm 2`; acknowledge it in **uatester's client** with the row's **確認** (Acknowledge) button | uatester shows acknowledged; **UaExpert** turns it Acknowledged at the same time |

### 1-9. History

Let the server run for at least 2 minutes:

| | Do | Expect |
|---|---|---|
| **UaExpert** | **Document → Add...** → **History Trend View**; drag `P0001_Temp` in, last 5 minutes, **Update** | A curve going up and down |
| **uatester client** | Tab **歷史** (History): node `ns=2;s=P0001_Temp`, read **原始值** (Raw values), last `5` minutes, **查詢** (Read) | Same curve shape as UaExpert; row count and time range agree |
| **uatester aggregate** | Read **平均** (Average), interval `60` s | One average per minute (the free UaExpert may not do aggregates; check uatester only) |

### 1-10. Secured connection and certificate trust

| | Do | Expect |
|---|---|---|
| **UaExpert** | Add `opc.tcp://127.0.0.1:4841`, endpoint **Basic256Sha256 - Sign & Encrypt**, Username `admin` / `admin`, Connect (trust the server certificate) | First attempt **refused** (`BadSecurityChecksFailed`, `BadCertificateUntrusted` or similar): the server does not trust UaExpert yet. Correct |
| **uatester client** | Disconnect, security `Basic256Sha256:SignAndEncrypt`, user admin, Connect | First attempt **refused** as well |
| **Trust** | uatester left half, tab **憑證** (Certificates): **重新整理** (Refresh), then **信任** (Trust) both certificates (UaExpert's and uatester's) | — |
| **Again** | Connect again on both sides and read `Setpoint` | Both connect and read the same value |
| **Other policy** | Both use **Aes256_Sha256_RsaPss - Sign & Encrypt** | Both succeed |

## Test 2: armv7 server (192.168.3.127, 32-bit)

Verifies the fix for 32-bit platforms (the field controllers). **The person starts the server on .127** (Computer use does not operate SSH):

```
chmod +x uatest-linux-armv7
./uatest-linux-armv7 server -addr 0.0.0.0:4841 -host 192.168.3.127 -points 100
```

Once it runs, use `opc.tcp://192.168.3.127:4841` on both sides (the IP: the network has no DNS) and repeat **1-1 to 1-7 and 1-10**. Differences:

- There is no uatester window on .127: in 1-7, instead of Send event, call Reset once from each side (the method fires an event); in 1-8, compare only the automatic `High` alarms and acknowledgement.
- Trust in 1-10: the person moves the files from `pki/rejected/certs/` to `pki/trusted/certs/` on .127.
- Watch 1-1: the old library dropped the connection on 32-bit (`BadTCPInternalError`); it should now connect.

## Reporting

One row per step; paste the table into the development conversation when done:

| Step | UaExpert | uatester client | Same? | Notes (required on failure or difference) |
|---|---|---|---|---|
| 1-1 | pass / fail | pass / fail | yes / no | |
| 1-2 | | | | |
| … | | | | |

On a failure or difference also record:
- The red lines in UaExpert's **Log** window around that step (whole lines).
- uatester's log lines from the same time and any error shown on screen.
- Screenshots of both sides at that moment.
