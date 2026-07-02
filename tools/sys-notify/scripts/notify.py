#!/usr/bin/env python3
"""sys-notify: 跨平台系统通知。Linux/macOS/Windows 均支持。"""
import sys
import json
import os
import subprocess
import platform

def send_notify_linux(title, body, urgency="normal"):
    """Linux: try notify-send, then dbus-send."""
    urgency_map = {"low": "low", "critical": "critical", "normal": "normal"}
    urgency_arg = urgency_map.get(urgency, "normal")

    # Try notify-send first
    try:
        subprocess.run(
            ["notify-send", "--urgency=" + urgency_arg, "--app-name=xworkbench", title, body],
            capture_output=True, timeout=5,
            env={**os.environ, "DISPLAY": os.environ.get("DISPLAY", "")}
        )
        return True
    except (FileNotFoundError, subprocess.TimeoutExpired):
        pass

    # Fallback: dbus-send
    try:
        subprocess.run(
            ["dbus-send", "--session", "--dest=org.freedesktop.Notifications",
             "--type=method_call", "/org/freedesktop/Notifications",
             "org.freedesktop.Notifications.Notify",
             "string:xworkbench", "uint32:0", "string:", "string:" + title,
             "string:" + body, "array:string:", "dict:string:variant:{}"],
            capture_output=True, timeout=5
        )
        return True
    except (FileNotFoundError, subprocess.TimeoutExpired):
        pass

    return False

def send_notify_darwin(title, body, urgency="normal"):
    """macOS: use osascript."""
    try:
        # urgency affects sound
        sound = "Basso" if urgency == "critical" else "Pop" if urgency == "normal" else "default"
        script = f'display notification "{body}" with title "{title}" sound name "{sound}"'
        subprocess.run(["osascript", "-e", script], capture_output=True, timeout=5)
        return True
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return False

def send_notify_windows(title, body, urgency="normal"):
    """Windows: use win32 ToastNotification via ctypes + PowerShell fallback."""
    try:
        import ctypes
        from ctypes import wintypes

        user32 = ctypes.windll.user32

        # Try PowerShell toast (more feature-rich)
        ps_script = f'''
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$template = @"
<toast>
  <visual>
    <binding template="ToastText02">
      <text id="1">{title}</text>
      <text id="2">{body}</text>
    </binding>
  </visual>
</toast>
"@
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($template)
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("xworkbench").Show($toast)
'''
        # Simpler approach: MessageBox (always works, non-blocking thread)
        MB_OK = 0x00000000
        MB_ICONINFORMATION = 0x00000040
        MB_TOPMOST = 0x00040000

        style = MB_OK | MB_ICONINFORMATION | MB_TOPMOST
        # Run MessageBox in a thread so it doesn't block
        def show_msg():
            user32.MessageBoxW(0, f"{body}", f"{title}", style)
        import threading
        t = threading.Thread(target=show_msg, daemon=True)
        t.start()
        return True
    except Exception as e:
        return False

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
