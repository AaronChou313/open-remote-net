#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
case "$(uname -s):$(uname -m)" in
 Darwin:arm64) target=darwin-arm64 ;;
 Linux:x86_64) target=linux-amd64 ;;
 *) printf '%s\n' '当前安装脚本支持 macOS arm64 和 Linux amd64'; exit 1 ;;
esac
binary="$root/dist/orn-agent-$target"
if [ ! -f "$binary" ]; then printf '%s\n' "请先构建或下载 $binary"; exit 1; fi
mkdir -p "$HOME/.local/bin"
install -m 755 "$binary" "$HOME/.local/bin/orn-agent"
if [ "$target" = darwin-arm64 ]; then
  config="$HOME/Library/Application Support/orn-agent/config.json"
  mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs/orn-agent"
  export ORN_INSTALL_HOME="$HOME"
  /usr/bin/python3 - <<'PYTHON'
import os,plistlib
from pathlib import Path
home=Path(os.environ['ORN_INSTALL_HOME'])
value={'Label':'top.openremotehub.orn','ProgramArguments':[str(home/'.local/bin/orn-agent'),'-config',str(home/'Library/Application Support/orn-agent/config.json')],'RunAtLoad':True,'KeepAlive':True,'WorkingDirectory':str(home),'StandardOutPath':str(home/'Library/Logs/orn-agent/service.log'),'StandardErrorPath':str(home/'Library/Logs/orn-agent/service.log'),'ThrottleInterval':10}
with (home/'Library/LaunchAgents/top.openremotehub.orn.plist').open('wb') as file:plistlib.dump(value,file)
PYTHON
  label="gui/$(id -u)/top.openremotehub.orn"
  if launchctl print "$label" >/dev/null 2>&1; then launchctl bootout "$label"; fi
  launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/top.openremotehub.orn.plist"
  "$HOME/.local/bin/orn-agent" -config "$config" open
else
  config="${XDG_CONFIG_HOME:-$HOME/.config}/orn-agent/config.json"
  mkdir -p "$HOME/.config/systemd/user"
  cat > "$HOME/.config/systemd/user/orn-agent.service" <<EOF
[Unit]
Description=OpenRemoteHub ORN
After=network-online.target
[Service]
ExecStart=%h/.local/bin/orn-agent -config "$config"
Restart=on-failure
RestartSec=5
[Install]
WantedBy=default.target
EOF
  systemctl --user daemon-reload
  systemctl --user enable --now orn-agent
  printf '%s\n' '已启动用户服务。运行 orn-agent open 打开本地页面。'
fi
