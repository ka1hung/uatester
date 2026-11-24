[繁體中文](uaexpert-test.md) | **English**

# Verifying the opcua server with UaExpert

Goal: connect the third-party client **UaExpert** (Unified Automation) to **uatester's server** (built on github.com/ka1hung/opcua) and confirm our OPC UA server works with industry tools. The client side was verified separately with uatest; this document tests the server only.

It is written for Claude operating UaExpert through Computer use in the Claude desktop app, and for the person watching. Each step says what to do and what should happen; report results in the format of the last section.

## Safety rules

- **Connect only to the test servers listed here** (local port 4841, 192.168.3.127:4841).
- **Do not connect to or write to** `192.168.3.205:46010` (the old MBController on site) or any MBController on port 4840.
- Use only uatester's built-in accounts `admin/admin` and `viewer/viewer`.

## Setup

| Item | Details |
|---|---|
| UaExpert | Installed on this Windows machine (the free edition is enough) |
| uatester desktop | `C:\Users\kaihung\Desktop\ccc\uatester\bin\uatester.exe` |
| Accounts | `admin` / `admin`: read, write, acknowledge alarms, call methods; `viewer` / `viewer`: read only; anonymous: read only |
| Test nodes | `ns=2;s=P0000_Temp` … `P0099_Temp` (Double, changing every 0.5 s), `ns=2;s=Setpoint` (Int16), `ns=2;s=Running` (Boolean), method `ns=2;s=Plant.Reset` (input Reason string, output Done Boolean) |
| Alarms | Every tenth point (P0000, P0010, P0020…) raises a `High` alarm above 80 on its own; alarms can also be raised by hand in uatester |

## Test 1: local server (Windows, 64-bit)

### 1-0. Start the server (uatester)

1. Open `uatester.exe`.
2. Left half, "伺服器" (Server): set **監聽位址** (listen address) to `0.0.0.0:4841` and **對外主機名稱 / IP** (host) to `127.0.0.1`; leave the rest.
3. Press "啟動伺服器" (Start server).
4. Expect: the state turns "執行中" (Running) and the log at the bottom shows 7 lines `endpoint url=opc.tcp://127.0.0.1:4841 ...`.

### 1-1. Add the server and connect anonymously

1. UaExpert **Project** pane → right-click **Servers** → **Add...**.
2. Under **Custom Discovery**, double-click **< Double click to Add Server... >**, enter `opc.tcp://127.0.0.1:4841`, OK.
3. Expand the new server and pick the **None - None (uatcp-uasc-uabinary)** endpoint.
4. **Authentication Settings**: **Anonymous**; press **OK**.
5. Right-click the server in the Project pane → **Connect**. If a certificate dialog appears, choose **Trust Server Certificate**, then **Continue**.
6. Expect: `Root` appears in the **Address Space** pane; no red errors in the **Log** window.

### 1-2. Browse

1. Address Space: expand **Objects → Plant**.
2. Expect: `P0000_Temp` … `P0099_Temp`, `Setpoint`, `Running`, `Reset`.
3. Select `P0000_Temp`; the **Attributes** pane shows DataType = Double, Description = simulated temperature.

### 1-3. Read and live updates

1. Drag `P0000_Temp`, `P0010_Temp`, `Setpoint`, `Running` into the **Data Access View** in the middle.
2. Expect: both temperatures change about every 0.5 s with Status Good; `Setpoint` = 22, `Running` = true.

### 1-4. Anonymous cannot write

1. In the Data Access View, double-click the **Value** of `Setpoint`, type `30`, Enter.
2. Expect: the write fails, the Log shows `BadUserAccessDenied`, the value stays 22.

### 1-5. Write as admin

1. Right-click the server → **Disconnect**.
2. Right-click → **Properties...**, Authentication Settings: **Username**, `admin` / `admin`, OK.
3. Right-click → **Connect**.
4. Drag `Setpoint` and `Running` into the Data Access View again if they are gone.
5. Set `Setpoint` to `30` and `Running` to `false`.
6. Expect: both writes succeed; uatester's log shows `client write user=admin node=ns=2;s=Setpoint value=30`.
7. Set `Setpoint` to `40000`. Expect: failure (out of Int16 range, `BadOutOfRange` or `BadTypeMismatch`).

### 1-6. Call a method

1. Address Space: Objects → Plant → right-click **Reset** → **Call...**.
2. Reason: `uaexpert test`; press **Call**.
3. Expect: result Good, output Done = true; uatester's log shows `method Reset user=admin reason=uaexpert test`.

### 1-7. Events

1. Menu **Document → Add...**, type **Event View**, OK.
2. Drag **Objects → Server** from the Address Space into the Event View's **Configuration** area.
3. Call Reset again (1-6).
4. Expect: an event `reset by admin: ...`; alarm events `P00x0_Temp high (> 80)` / `back to normal` appear over time (some points take tens of seconds to pass 80).

### 1-8. Alarms & Conditions

1. **Document → Add...**, type **Alarms & Conditions**, OK.
2. Drag **Objects → Server** into it (or press its Refresh icon).
3. Expect: active `High` alarms (Severity 700), including those already active before connecting (ConditionRefresh).
4. In uatester: left half, tab "警報與事件" (Alarms & events): point `P0005_Temp`, severity `900`, message `uaexpert manual`, press "觸發警報" (Raise alarm).
5. Expect: the alarm appears in UaExpert's Alarms view **at once** (Severity 900).
6. In UaExpert, right-click the alarm → **Acknowledge**, comment `ok`, confirm.
7. Expect: its AckedState becomes Acknowledged; in uatester the row shows "已確認" (acknowledged) and the log shows `client acknowledge user=admin`.

### 1-9. History

1. Let the server run for at least 2 minutes.
2. **Document → Add...**, type **History Trend View**, OK.
3. Drag `P0001_Temp` in, set the range to the last 5 minutes, press **Update**.
4. Expect: a curve going up and down.

### 1-10. Secured connection and certificate trust

1. Project pane → right-click Servers → Add..., `opc.tcp://127.0.0.1:4841` again, this time the **Basic256Sha256 - Sign & Encrypt** endpoint, Username `admin` / `admin`.
2. Connect. When UaExpert shows the server certificate, choose **Trust Server Certificate** → Continue.
3. Expect: the first attempt is **refused** (the Log shows something like `BadSecurityChecksFailed` or `BadCertificateUntrusted`): the server does not trust UaExpert's certificate yet. This is correct.
4. In uatester: left half, tab "憑證" (Certificates): UaExpert's certificate is listed; press "信任" (Trust).
5. Connect again in UaExpert. Expect: connected; reading and writing work.
6. Repeat with **Aes256_Sha256_RsaPss - Sign & Encrypt**.

## Test 2: armv7 server (192.168.3.127, 32-bit)

This part verifies the fix for 32-bit platforms (the field controllers). **The person starts the server on .127** (Computer use does not operate SSH):

```
chmod +x uatest-linux-armv7
./uatest-linux-armv7 server -addr 0.0.0.0:4841 -host 192.168.3.127 -points 100
```

Once it runs:

1. In UaExpert add `opc.tcp://192.168.3.127:4841` (use the IP, not a computer name: the network has no DNS).
2. Repeat **1-1 to 1-8** and **1-10** of Test 1 (in 1-10, "Trust" means the person moves the file from `pki/rejected/certs/` to `pki/trusted/certs/` on .127).
3. Watch 1-1 in particular: the old library dropped the connection on 32-bit (`BadTCPInternalError`); it should now connect.

## Reporting

One row per step; paste the table back into the development conversation when done:

| Step | Result | Notes (required on failure) |
|---|---|---|
| 1-1 | pass / fail | Full error lines from the Log window, a screenshot of the moment |
| 1-2 | | |
| … | | |

On a failure also record:
- The red lines in UaExpert's **Log** window around that step (copy whole lines).
- uatester's log lines from the same time.
- A screenshot at the moment it failed.
