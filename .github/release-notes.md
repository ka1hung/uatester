## 下載 / Downloads

| 檔案 File | 說明 | Description |
|---|---|---|
| `uatester-*-windows-amd64.zip` | 桌面版，Windows 10 / 11 | Desktop app, Windows 10 / 11 |
| `uatester-*-macos-universal.zip` | 桌面版，macOS 12 以上（Intel 與 Apple Silicon） | Desktop app, macOS 12+ (Intel and Apple Silicon) |
| `uatester-*-linux-amd64.tar.gz` | 桌面版，Linux（需要 GTK 3 與 WebKitGTK 4.1） | Desktop app, Linux (needs GTK 3 and WebKitGTK 4.1) |
| `uatest-*-<系統>-<架構>` | 命令列版，不需要任何相依套件；`linux-armv7` 給工控機 | Command line tool, no dependencies; `linux-armv7` for field controllers |
| `SHA256SUMS.txt` | 檔案校驗碼 | Checksums |

**macOS**：程式沒有經過 Apple 簽章公證。第一次開啟請在 Finder 對 `uatester.app` 按右鍵 →「打開」，或在終端機執行 `xattr -dr com.apple.quarantine uatester.app`。
The app is not notarized by Apple. The first time, right-click `uatester.app` in Finder and choose Open, or run `xattr -dr com.apple.quarantine uatester.app`.

**Linux 桌面版 / Linux desktop**：`sudo apt install libgtk-3-0 libwebkit2gtk-4.1-0`（Ubuntu 22.04 以上、Debian 12 以上 / Ubuntu 22.04+, Debian 12+）。

使用說明見 [README](https://github.com/ka1hung/uatester#readme)。 See the [README](https://github.com/ka1hung/uatester/blob/main/README.en.md) for usage.
