[繁體中文](README.md) | **English**

# uatester

An OPC UA test tool: a simulated server and a client that test each other, or test other OPC UA products (UaExpert, SCADA, PLCs). Built on [ka1hung/opcua](https://github.com/ka1hung/opcua).

Two versions share one core (`core/`):

| Version | Binary | Where |
|---|---|---|
| Desktop | `uatester.exe` (Wails v3) | Windows laptop: server and client in one window |
| Command line | `uatest` | Headless controllers (Linux armv7) for soak tests; quick diagnostics in the field |

## Desktop

The **server** is on the left, the **client** on the right, and a log shared by both at the bottom.

**Server**
- Set the listen address, point count, update interval and history depth, then press Start server.
- Points: values follow a simulation; set any point by hand (the simulation then leaves it alone until Resume), or pause the whole simulation.
- Alarms & events: every tenth point raises a High alarm above 80 on its own; you can also raise or clear alarms by hand, acknowledge them and send events.
- Connected clients: who is connected, as which user, with how many subscriptions.
- Certificates: clients that tried a secured connection and are not trusted yet are listed here; press Trust.
- Users: `admin/admin` reads, writes and acknowledges; `viewer/viewer` and anonymous only read.

**Client**
The client covers what the server offers, so it tests this tool's server as well as other vendors' servers.

- Connect: enter the endpoint URL, pick security and user. "Endpoints" lists the security and logins the server offers; click one to use it. "Connect client here" in the server's endpoint list works too. Once connected, the server's product, version and state are shown.
- Browse: the node tree. A picked node shows its description, data type and what this user may do (read, write, history, execute).
  - Variables: read on pick; write, watch, or look at history.
  - Methods: a field per argument from the method's definition; Call shows the outputs.
  - Objects with events: subscribe to their events or read their event history.
- Watch: subscribe to several nodes and see values change live, with an optional deadband (absolute or percent).
- Alarms & events: subscribe to the Server object or an area. Current alarms lists active and unacknowledged alarms, with Acknowledge; alarms already active before connecting are fetched too. The event log lists the latest events.
- History: raw values or aggregates (average, minimum, maximum, count, first / last in interval) as a line chart and a table, and event history.
- Load test: many clients subscribe to this tool's server points at once; notifications per second are measured.

The server's and the client's certificates live in `%APPDATA%\uatester`, so they survive moving the program, and other products trust them only once.

The interface switches between Traditional Chinese and English, and follows Windows' light / dark setting.

## Command line

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

- Security: `-security none` (default), `auto` (Sign or better, never None), or `policy:mode` such as `Basic256Sha256:SignAndEncrypt`.
- `-pki folder` keeps this tool's client certificate, so a server trusts it once.
- `uatest <command> -h` lists the command's flags.

**armv7 soak test** (on the controller):

```
uatest server -addr 0.0.0.0:4840 -host <IP> -points 3000 -interval 500ms -history 0 -stats 10m > soak.log 2>&1
# on another machine
uatest stress -ep opc.tcp://<IP>:4840 -clients 10 -points 3000 -interval 500ms -duration 72h
```

Reading it: `goroutines`, `heapMB` and `sysMB` in `soak.log` must not keep rising over the 72 hours.

## Building

Needs Go 1.26 or later. The desktop version needs Windows and WebView2 (built into Windows 10 / 11); no Node.js or C compiler. The commands below are for Git Bash; in PowerShell set environment variables as `$env:GOOS="linux"`.

```
# desktop (Windows)
go build -tags production -trimpath -ldflags "-H windowsgui -s -w" -o bin/uatester.exe ./app

# command line
go build -o bin/uatest.exe ./cmd/uatest
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o bin/uatest-armv7 ./cmd/uatest
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/uatest-linux ./cmd/uatest
```

While developing, the desktop interface can run in a browser (fully working, just served to a browser):

```
go build -tags server -o bin/uatester-web.exe ./app
WAILS_SERVER_PORT=8080 bin/uatester-web.exe      # then open http://localhost:8080
```

## Testing

```
go test ./...
```

The `core` tests start the simulated server and connect a real client: read/write permissions, values set by hand, history, an alarm raised by the server then received and acknowledged by the client, method calls, events, certificate trust, load test.

## Layout

| Path | Contents |
|---|---|
| `core/` | Shared core: simulated server, in-memory history, certificates, client connection, history reads, load test |
| `cmd/uatest/` | Command line version |
| `app/` | Desktop version: Go services (`*_service.go`) and interface (`frontend/`, plain HTML / CSS / JS, no build step) |

## Dependencies

Uses `github.com/ka1hung/opcua` (a private repository). Before building:

```
go env -w GOPRIVATE=github.com/ka1hung/*
```

and let git reach private GitHub repositories (`gh auth setup-git` or an SSH key). To work against the opcua checkout next to this one, add `replace github.com/ka1hung/opcua => ../opcua` for a while, and remove it before committing.

## License

MIT.
