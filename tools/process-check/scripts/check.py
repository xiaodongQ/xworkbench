#!/usr/bin/env python3
"""process-check: 检查进程是否存活，返回 PID/CPU/内存信息"""
import sys
import json
import os
import platform
import subprocess
import re

def check_process_linux(name):
    """Linux: use ps to find process."""
    try:
        # Get pid, cpu, mem, cmd for processes matching name
        output = subprocess.check_output(
            ["ps", "aux"],
            text=True, timeout=5
        )
    except subprocess.TimeoutExpired:
        return None, "ps command timeout"
    except FileNotFoundError:
        return None, "ps not found"

    lines = output.strip().split('\n')
    header = lines[0].lower()
    # Find column indices
    cols = header.split()
    pid_idx = cols.index('pid') if 'pid' in cols else 1
    cmd_idx = len(cols) - 1

    matches = []
    for line in lines[1:]:
        parts = line.split(None, len(cols) - 1)
        if len(parts) < 2:
            continue
        cmd = ' '.join(parts[cmd_idx:]) if cmd_idx < len(parts) else parts[-1]
        if name.lower() in cmd.lower() or name.lower() in os.path.basename(cmd).lower():
            matches.append({
                "pid": parts[pid_idx] if pid_idx < len(parts) else "0",
                "cpu": parts[2] if len(parts) > 2 else "0",
                "mem": parts[3] if len(parts) > 3 else "0",
                "cmdline": cmd,
            })

    if not matches:
        return None, None  # not found

    # Return first match (most likely the main process)
    m = matches[0]
    try:
        cpu = float(m["cpu"])
        mem = float(m["mem"])
    except ValueError:
        cpu = mem = 0.0

    return {
        "status": "running",
        "pid": int(m["pid"]),
        "cpu_percent": round(cpu, 1),
        "mem_percent": round(mem, 1),
        "cmdline": m["cmdline"],
    }, None

def check_process_darwin(name):
    """macOS: use ps with BSD format."""
    try:
        output = subprocess.check_output(
            ["ps", "aux"],
            text=True, timeout=5
        )
    except (subprocess.TimeoutExpired, FileNotFoundError) as e:
        return None, str(e)

    for line in output.strip().split('\n')[1:]:
        parts = line.split()
        if len(parts) < 4:
            continue
        cmd = ' '.join(parts[4:])
        if name.lower() in cmd.lower():
            return {
                "status": "running",
                "pid": int(parts[1]),
                "cpu_percent": round(float(parts[2]), 1),
                "mem_percent": round(float(parts[3]), 1),
                "cmdline": cmd,
            }, None
    return None, None

def check_process_windows(name):
    """Windows: use tasklist."""
    try:
        output = subprocess.check_output(
            ["tasklist", "/FO", "CSV", "/NH"],
            text=True, timeout=10, shell=True
        )
    except subprocess.TimeoutExpired:
        return None, "tasklist timeout"
    except FileNotFoundError:
        return None, "tasklist not found"

    for line in output.strip().split('\n'):
        parts = line.strip().split(',')
        if len(parts) < 2:
            continue
        pname = parts[0].strip('"')
        pid = parts[1].strip('"')
        if name.lower() in pname.lower():
            return {
                "status": "running",
                "pid": int(pid),
                "cmdline": pname,
            }, None
    return None, None

def main():
    try:
        params = json.load(sys.stdin)
    except Exception:
        params = {}

    name = params.get("name", "").strip()
    if not name:
        print(json.dumps({"status": "error", "error": "name is required"}))
        sys.exit(1)

    system = platform.system()
    if system == "Linux":
        result, err = check_process_linux(name)
    elif system == "Darwin":
        result, err = check_process_darwin(name)
    elif system == "Windows":
        result, err = check_process_windows(name)
    else:
        result, err = None, f"unsupported platform: {system}"

    if err:
        print(json.dumps({"status": "error", "error": err}))
        sys.exit(1)
    if result is None:
        print(json.dumps({"status": "not_found", "error": f"process '{name}' not found"}))
        sys.exit(0)  # not found is valid result

    print(json.dumps(result))

if __name__ == "__main__":
    main()
