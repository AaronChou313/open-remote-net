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

## 实施状态

M1 已完成首轮真实验收。Mac 节点加入网络后能看到 Windows。用户已确认通过 ORN 转发的原生 RustDesk 窗口看到并操作 Windows 桌面。ECS 两个隔离节点在直连与强制 DERP 模式下分别完成双向 114,688 字节校验，DERP region 999；断开测试容器网络后原进程约 1.48 秒恢复传输。Mac / Windows / Linux 二进制已编译。远端 Web 入口、ORH 集成、Cloud 隔离和 Android 正在继续开发。
