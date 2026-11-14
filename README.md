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

macOS / Linux 执行 `sh agent/packaging/install.sh`。macOS 安装用户 LaunchAgent `top.openremotehub.orn`，Linux 安装用户 systemd 服务 `orn-agent`。Windows 在 PowerShell 执行 `agent/packaging/install.ps1`，安装当前用户登录触发的计划任务。Linux 用户服务的开机常驻依赖系统已有的用户登录 / linger 设置。

## 数据路径

控制服务为 `https://hs.openremotehub.top`。连接优先直连，失败时通过该服务的 DERP 中继，**中继流量会经过 ECS 并消耗带宽**。Cloud API 负责账号、绑定与设备目录。userspace 核心沿用宿主路由，兼容代理 FakeIP DNS。日志仅写入本机，不上传到上游厂商。

本地转发默认由系统分配端口，返回 `127.0.0.1:<port>`，通过 `net.Conn` 双向传输。RustDesk 保留原生认证与桌面权限流程。

## Cloud 绑定与控制台

首次启动后运行 `orn-agent open`，点击“绑定账号”，在 Cloud 确认绑定码。注册使用管理员签发的一次性邀请码。入网凭据与设备 UUID 保存在私有配置目录，后续登录和心跳自动恢复。

与 ORH 共址时，ORH 自动通过 LocalAPI 将已有 Cloud 绑定交给 ORN，沿用同一 UUID。ORH 负责地址和心跳，ORN 负责入网与节点对账。ORH 管理页通过 `ORH_ORN_API_URL` 和 `ORH_ORN_API_TOKEN_FILE` 读取本机 ORN。

控制端先启动 ORN，再从 Cloud 点击设备的“打开控制台”。本机会创建该设备专用的 `http://orh-<UUID>.localhost:<port>`，通过 userspace 网络转发 HTTP 和 WebSocket，Cloud 登录票据仍单次兑换。Zellij 与 DSH 继续使用原生页面。直接局域网访问可使用“直接访问”。

userspace 网络提供进程内连接。其他应用的系统网络路由继续由宿主机管理。VS Code Remote SSH、其他 Web 服务可使用已有可达地址，或在 ORN 配置 `publish` 后使用“端口转发”。默认端口需在实际主机确认。RustDesk 客户端由用户单独安装，并保留其认证流程。

## Android

下载 Release 中的 `orn-android-arm64-debug.apk`，在手机或平板侧载安装。包名为 `top.openremotehub.orn`，最低 Android 8。打开后绑定相同 Cloud 账号，选择设备进入控制台或远程桌面。远程桌面通过 Intent 打开设备上另行安装的官方 RustDesk。

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
- M2 已切换 LocalAPI。真实 Cloud 登录进入 Mac 控制台，Zellij 命令执行及重连、Docker、文件浏览和 DSH 均可用。断开 Mac 原生网络客户端后，ORN 和控制台仍可使用。原客户端已恢复连接，供 Windows 安装 ORN 前过渡使用。
- M3 已通过真实双账号、各两节点隔离。账号内 TCP 正常，跨账号原始 Dial 超时。解绑删除实际节点，原进程重新绑定后恢复转发。删除测试账号同步清理其全部网络节点。headscale 重启后 Cloud socket 仍可访问。
- M4 已生成 debug APK，通过 Go race tests、Android 构建与 lint。Android 15 arm64 模拟器完成真实 Cloud 绑定、账号隔离、7,680 字节 TCP 回环及 WiFi 关闭/恢复，原进程在约 3.1 秒恢复连接。

Vivo Pad Air、实际蜂窝/校园网、真机 RustDesk Intent、后台省电行为、横屏视觉，以及 Windows ORN 安装和移除旧客户端后的整体验收仍需在对应设备完成。Android 模拟器的网络与服务已验证，原生页面尚未完成视觉验收。

上游 Go 与依赖许可随安装包提供 `THIRD_PARTY_NOTICES.txt`，Android APK 也内置该文件。源码中的软件名称与许可证归各自权利人。
