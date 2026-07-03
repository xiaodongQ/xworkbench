#!/usr/bin/env python3
"""
sys-notify: 跨平台系统通知。Linux/macOS/Windows 均支持。
支持 urgency 低/普通/紧急 三档，通知弹窗更美观。
"""
import sys
import json
import os
import subprocess
import platform

# 通知超时（毫秒）
EXPIRE_MS = 5000
URGENCY_HINT = {"low": 0, "normal": 1, "critical": 2}


# ── Linux ────────────────────────────────────────────────────────────────────
def send_notify_linux(title, body, urgency="normal"):
    """Linux: notify-send（支持图标、行分隔、超时、urgency）。"""
    # urgency 参数映射
    urgency_arg = {"low": "low", "normal": "normal", "critical": "critical"}.get(urgency, "normal")
    expire_ms = 8000 if urgency == "critical" else (3000 if urgency == "low" else EXPIRE_MS)

    # 尝试 notify-send（最常用，界面最美观）
    try:
        # 将 body 按换行符拆分，notify-send 的 body 部分支持简单标记
        lines = body.split("\n")
        # 第一行作为 body，第二行开始作为副文本（部分桌面支持）
        body_line = lines[0] if lines else ""
        hint_line = "\n".join(lines[1:]) if len(lines) > 1 else ""

        cmd = [
            "notify-send",
            "--urgency=" + urgency_arg,
            "--app-name=xworkbench",
            "--expire-time=" + str(expire_ms),
            "--icon=dialog-information",
            title,
        ]
        if hint_line:
            # 某些 notify-daemon（如 dunst/gnome）支持 body 分两行
            cmd[-1] = title + "\n" + body_line
            cmd.append(hint_line)
        else:
            cmd.append(body_line)

        subprocess.run(
            cmd,
            capture_output=True,
            timeout=6,
            env={**os.environ, "DISPLAY": os.environ.get("DISPLAY", "")},
        )
        return True
    except (FileNotFoundError, subprocess.TimeoutExpired):
        pass

    # Fallback: dbus-send（直接调 freedesktop DBus，通用性最强）
    try:
        lines = body.split("\n", 1)
        body_str = lines[0]
        hint_str = lines[1] if len(lines) > 1 else ""

        subprocess.run(
            [
                "dbus-send", "--session",
                "--dest=org.freedesktop.Notifications",
                "--type=method_call",
                "/org/freedesktop/Notifications",
                "org.freedesktop.Notifications.Notify",
                "string:xworkbench",
                "uint32:0",
                "string:dialog-information",
                "string:" + title,
                "string:" + body_str,
                "array:string:",
                f"dict:string:variant:expire_timeout:{expire_ms}",
            ],
            capture_output=True,
            timeout=6,
        )
        return True
    except (FileNotFoundError, subprocess.TimeoutExpired):
        pass

    return False


# ── macOS ────────────────────────────────────────────────────────────────────
def send_notify_darwin(title, body, urgency="normal"):
    """macOS: osascript 原生通知（自带系统样式）。"""
    # 拆行：第一行正文，第二行 subtitle（Finder/通知中心支持）
    lines = body.split("\n", 1)
    body_part = lines[0]
    subtitle_part = lines[1] if len(lines) > 1 else ""

    sound_map = {"low": "default", "normal": "Pop", "critical": "Basso"}
    sound = sound_map.get(urgency, "Pop")

    if subtitle_part:
        # macOS 12+: notification with subtitle
        script = (
            f'display notification "{_esc(body_part)}" '
            f'with title "{_esc(title)}" '
            f'subtitle "{_esc(subtitle_part)}" '
            f'sound name "{sound}"'
        )
    else:
        script = (
            f'display notification "{_esc(body_part)}" '
            f'with title "{_esc(title)}" '
            f'sound name "{sound}"'
        )

    try:
        subprocess.run(["osascript", "-e", script], capture_output=True, timeout=6)
        return True
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return False


# ── Windows ─────────────────────────────────────────────────────────────────
def send_notify_windows(title, body, urgency="normal"):
    """Windows: PowerShell ToastNotification（现代化通知弹窗）。"""
    lines = body.split("\n", 1)
    body_part = lines[0]
    hint_part = lines[1] if len(lines) > 1 else ""

    # urgency 影响通知时长和样式
    duration = "long" if urgency == "critical" else "short"

    ps_script = f'''
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$xml = @"
<toast duration="{duration}">
  <visual>
    <binding template="ToastGeneric">
      <text>{_esc_ps(title)}</text>
      <text>{_esc_ps(body_part)}</text>
      <text hint-style="captionSubtle" hint-wrap="true">{_esc_ps(hint_part)}</text>
    </binding>
  </visual>
  <audio src="ms-winsoundevent:Notification.{'Default' if urgency != 'critical' else 'Looping.Alarm'}" />
</toast>
"@
$doc = New-Object Windows.Data.Xml.Dom.XmlDocument
$doc.LoadXml($xml)
$toast = [Windows.UI.Notifications.ToastNotification]::new($doc)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("xworkbench").Show($toast)
'''

    try:
        proc = subprocess.run(
            ["powershell", "-NoProfile", "-NonInteractive", "-Command", ps_script],
            capture_output=True, timeout=8,
        )
        if proc.returncode == 0:
            return True
    except (FileNotFoundError, subprocess.TimeoutExpired):
        pass

    # Fallback: 静默失败，不回退到丑陋的 MessageBox
    return False


# ── 辅助 ─────────────────────────────────────────────────────────────────────
def _esc(s):
    """转义 AppleScript 特殊字符。"""
    return s.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n")


def _esc_ps(s):
    """转义 PowerShell 字符串中的 XML 特殊字符。"""
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace('"', "&quot;").replace("'", "&apos;")


# ── 主入口 ───────────────────────────────────────────────────────────────────
def main():
    try:
        params = json.load(sys.stdin)
    except Exception:
        params = {}

    title = params.get("title", "").strip()
    body = params.get("body", "").strip()
    urgency = params.get("urgency", "normal")

    if not title or not body:
        print(json.dumps({"status": "error", "error": "title and body are required"}))
        sys.exit(1)

    system = platform.system()
    sent = False

    if system == "Linux":
        sent = send_notify_linux(title, body, urgency)
    elif system == "Darwin":
        sent = send_notify_darwin(title, body, urgency)
    elif system == "Windows":
        sent = send_notify_windows(title, body, urgency)

    if sent:
        print(json.dumps({"status": "sent"}))
    else:
        print(json.dumps({"status": "error", "error": f"notification not supported on {system}"}))
        sys.exit(1)


if __name__ == "__main__":
    main()
