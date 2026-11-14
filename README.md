# Open Remote Net

ORN 是 OpenRemoteHub 的组网子系统。Go 单二进制使用 userspace 网络，提供本地页面、设备状态、受控端口转发与 RustDesk 入口。源码依赖固定为 tsnet 1.104.7，需要 Go 1.27.4。

## 构建与运行

```sh
go test -race ./agent/internal/...
sh agent/packaging/build.sh
TS_AUTHKEY='<短期一次性密钥>' ./dist/orn-agent-darwin-arm64
./dist/orn-agent-darwin-arm64 open
```

`open` 通过 URL fragment 将本机私有凭据交给页面，页面随即清除 fragment。程序接口从 `api.token` 读取 Bearer token。LocalAPI 仅监听 `127.0.0.1:4789`，校验 Host、Origin 和访问凭据。

配置默认保存在 macOS `~/Library/Application Support/orn-agent/`、Linux `$XDG_CONFIG_HOME/orn-agent/`（默认 `~/.config/orn-agent/`）、Windows `%APPDATA%/orn-agent/`。`-config` 指定独立状态目录。首次入网后使用 state 中持久化的身份，重启时可移除 `TS_AUTHKEY`。

```json
{
  "control_url": "https://hs.openremotehub.top",
  "hostname": "my-device",
  "state_dir": "/absolute/private/directory/state",
  "api_addr": "127.0.0.1:4789",
  "cloud": {"base_url": "https://cloud.openremotehub.top"},
  "publish": {"21118": "127.0.0.1:21118"}
}
```

`publish` 将指定的本机服务提供给通过 ACL 授权的网络设备。Mac 与当前 Windows 的 RustDesk 直连端口已实测为 **21118**。其他主机需按实际监听端口配置。关闭本地页面后进程与转发继续运行。无活动连接的转发在 10 分钟后移除，活动连接也有空闲读超时。

## 用户服务

与 ORH 同机部署时不需要单独安装本服务：ORH 的安装脚本会把二进制放入 `bin/`，由控制台进程启动并监护，设备身份与 Cloud 绑定沿用标准配置目录。下面的独立安装方式适用于只运行 ORN 的主机。

macOS / Linux 执行 `sh agent/packaging/install.sh`。macOS 安装用户 LaunchAgent `top.openremotehub.orn`，Linux 安装用户 systemd 服务 `orn-agent`。Windows 解压安装包后，在 PowerShell 执行 `powershell -NoProfile -ExecutionPolicy Bypass -File .\agent\packaging\install.ps1`。它安装当前用户登录触发的计划任务及开始菜单入口“OpenRemoteHub ORN”。Linux 用户服务的开机常驻依赖系统已有的用户登录 / linger 设置。

## 数据路径

控制服务为 `https://hs.openremotehub.top`。连接优先直连，失败时通过该服务的 DERP 中继，**中继流量会经过 ECS 并消耗带宽**。Cloud API 负责账号、绑定与设备目录。userspace 核心沿用宿主路由，兼容代理 FakeIP DNS。日志仅写入本机，不上传到上游厂商。

本地转发默认由系统分配端口，返回 `127.0.0.1:<port>`，通过 `net.Conn` 双向传输。RustDesk 保留原生认证与桌面权限流程。

## Cloud 绑定与控制台

首次启动后运行 `orn-agent open`，点击“绑定账号”，在 Cloud 确认绑定码。注册使用管理员签发的一次性邀请码。入网凭据与设备 UUID 保存在私有配置目录，后续登录和心跳自动恢复。

与 ORH 共址时，ORH 自动通过 LocalAPI 将已有 Cloud 绑定交给 ORN，沿用同一 UUID。ORH 负责地址和心跳，ORN 负责入网与节点对账。ORH 管理页通过 `ORH_ORN_API_URL` 和 `ORH_ORN_API_TOKEN_FILE` 读取本机 ORN。

同一台电脑不要同时用系统 Tailscale 登录该控制服务：会留下第二个节点，它没有 ORH 控制台，打开控制台会提示“目标控制台身份不匹配”。退出系统 Tailscale 或删除旧节点即可。

控制端先启动 ORN，再从 Cloud 点击设备的“打开控制台”。本机会创建该设备专用的 `http://orh-<UUID>.localhost:<port>`，通过 userspace 网络转发 HTTP 和 WebSocket，Cloud 登录票据仍单次兑换。Zellij 与 DSH 继续使用原生页面。直接局域网访问可使用“直接访问”。

userspace 网络提供进程内连接。其他应用的系统网络路由继续由宿主机管理。VS Code Remote SSH、其他 Web 服务可使用已有可达地址，或在 ORN 配置 `publish` 后使用“端口转发”。默认端口需在实际主机确认。RustDesk 客户端由用户单独安装，并保留其认证流程。

## Android

下载 Release 中的 `orn-android-arm64-debug.apk`，在手机或平板侧载安装。应用显示名为 ORH，包名保持 `top.openremotehub.orn` 以便覆盖升级保留绑定，最低 Android 8。打开后绑定相同 Cloud 账号，设备列表来自本机 ORN 的组网视图。

控制台在应用内 WebView 打开：Go 核为每台设备建立独立 `.localhost` 浏览器来源，会话 Cookie、终端 WebSocket 与 Cloud 单次票据都在同一视图内完成，可随时切到外部浏览器。远程桌面仍通过 Intent 打开设备上另行安装的官方 RustDesk，本机 ORH 与平板使用同一套入口。

应用使用前台连接服务和持续通知，网络变化触发重连。按设备系统设置允许通知及后台运行。正式签名由项目所有者完成，debug APK 用于验收。

```sh
# Go 1.27.4、JDK 21、Android SDK 35、NDK 28.2.13676358
export ANDROID_HOME=/absolute/path/to/android-sdk
export ANDROID_NDK_HOME="$ANDROID_HOME/ndk/28.2.13676358"
sh agent/packaging/build-android.sh
```

Go 核心复用 desktop 的 mesh、Cloud、LocalAPI 和转发逻辑。Android 使用系统 LinkProperties 提供真实接口与默认路由，日志写入应用私有目录。手机单列、平板宽屏双列，跟随系统深浅色。应用状态和凭据禁止系统备份与设备迁移。

## 实测与验收范围

- M1 已通过。用户确认 Mac 经 ORN 操作 Windows 桌面。ECS 两个隔离节点在 direct 与强制 DERP 模式分别完成双向 114,688 字节传输，region 999。断开容器网络后原进程约 1.48 秒恢复。
- M2 已切换 LocalAPI。真实 Cloud 登录进入 Mac 控制台，Zellij 命令执行及重连、Docker、文件浏览和 DSH 均可用。断开 Mac 原生网络客户端后，ORN 和控制台仍可使用。Windows 用户已确认安装绑定后远程桌面连接正常。迁移验收期间保留原客户端。
- M3 已通过真实双账号、各两节点隔离。账号内 TCP 正常，跨账号原始 Dial 超时。解绑删除实际节点，原进程重新绑定后恢复转发。删除测试账号同步清理其全部网络节点。headscale 重启后 Cloud socket 仍可访问。
- M4 已生成 debug APK，通过 Go race tests、Android 构建与 lint。Android 15 arm64 模拟器完成真实 Cloud 绑定、账号隔离、7,680 字节 TCP 回环及 WiFi 关闭/恢复，原进程在约 3.1 秒恢复连接。

Vivo Pad Air、实际蜂窝/校园网、真机 RustDesk Intent、后台省电行为、横屏视觉，以及 Windows ORN 安装和移除旧客户端后的整体验收仍需在对应设备完成。Android 模拟器的网络与服务已验证，原生页面尚未完成视觉验收。

上游 Go 与依赖许可随安装包提供 `THIRD_PARTY_NOTICES.txt`，Android APK 也内置该文件。源码中的软件名称与许可证归各自权利人。


### Android 0.1.1 修复

RustDesk 深链保留 `127.0.0.1:端口` 中的字面冒号，兼容 RustDesk 直接读取 Dart `Uri.path` 的行为。Android SDK instrumentation 已复现旧链接的 `%3A` 并验证新链接。版本码递增，可直接覆盖安装并保留绑定；平板实际桌面连接等待更新后复测。

### Android 0.1.2 连接保持

同一设备的远程桌面复用已有转发，并保存本地端口，核心重启后再次进入仍使用该端口。RustDesk 的“记住密码”因此可以关联稳定入口；密码由 RustDesk 自己保存。

首次进入远程桌面时，Android 会请求允许 ORN 后台运行。允许后继续打开 RustDesk，也可从 ORN 的“允许后台连接”进入系统设置。有活动转发时持有限时、持续续期的 partial wake lock，连接结束后释放。部分厂商还需在系统电池设置中允许后台高耗电。此授权由 Android 管理。[Android 后台网络说明](https://developer.android.com/training/monitoring-device-state/doze-standby)

可选的 SDK instrumentation 测试接受 `-e peer <测试节点IP>`，测试节点需在组网端口 21118 发布隔离的 TCP echo 服务。测试会验证重复进入、150 秒后台传输、连接释放及核心重启后的端口复用。不得将真实 RustDesk 端口用于该 echo 测试。

### Android 0.1.3 断连诊断与自恢复

- 远程桌面转发在进程被系统终止后自动重建：沿用保存的同一 `127.0.0.1` 端口，并发出“远程桌面已恢复”通知，点击即可重新打开 RustDesk，无需在 ORN 中重新发起。
- 有活动转发时同时持有 Wi-Fi 高性能锁与 CPU 唤醒锁，并保留前台服务与 15 秒心跳；系统或厂商清理进程后由 `AlarmManager` 看门狗尝试拉起。
- 仅在默认网络、网关或接口列表实际变化时才通知组网内核重绑，避免无关的链路属性回调反复切换路径。
- 页面显示内核运行时长和活动连接数。断连后运行时长重新计数即表示进程被系统重启；转发仍在但没有连接则表示链路中断。
- 新增 `GET /api/v1/diagnostics`（仅回环、需本机凭据）：进程运行时长、协程数、组网状态、每个转发的实时连接数、上下行字节数与最近 200 条连接事件。所有结果只在本机生成，不经过 Cloud。
- Android 端记录事件到应用私有 `diagnostics.log`（128 KB 循环，最多保留两份），页面“连接诊断”可查看并一键复制；“厂商后台设置”可跳转 vivo 等系统的自启动 / 后台高耗电页面。

排查步骤：断连后立刻打开 ORN，点“连接诊断”→“复制”，把文本发回。若诊断显示内核运行时长很短，说明是系统终止了后台进程（需在厂商设置中允许后台运行）；若内核运行很久且组网正常、只是没有连接，则是网络链路问题，需要同时查看对端与中继日志。

### Android 0.2.1 设备能力标记

- 设备列表读取 Cloud 目录中每台设备发布的组网服务；只提供远程桌面（没有 8765 控制台）的设备显示“仅远程桌面”，控制台按钮禁用，避免点击后才报错。
- Cloud 目录每分钟最多刷新一次，目录不可用时保持原有可用状态，不阻塞设备列表。
- 目标设备若没有 ORH 控制台，错误文案会说明可能是旧组网节点或 ORH 未运行。

### Android 0.2.0 ORH 客户端

- 应用名称改为 ORH，页面与通知统一使用 ORH 标识；包名不变，可直接覆盖 0.1.x 并保留账号绑定。
- 控制台改为应用内 WebView：每设备 `.localhost` 来源、Cookie、DOM 存储与 WebSocket 终端保持一致；顶部提供刷新与“浏览器”切换，外部链接交给系统浏览器。
- 控制台连接中断时显示可操作的重试页，重试会重新向本机 ORN 申请该设备的新转发地址（内核重启后端口变化也能恢复）。
- 设备列表沿用组网视图，新增的控制台入口与电脑端 ORH 的“控制台”动作一致。

已验证 Android 15 平板模拟器：应用内 WebView 能解析 `*.localhost` 到 127.0.0.1 并执行页面脚本，页面装帧避开系统状态栏；构建与 lint 通过。真实 Cloud 票据跳转与平板端触控仍在真机复测。
