#!/usr/bin/env python3
"""dir-backup: 备份目录到带时间戳的 tar.gz"""
import sys
import json
import os
import tarfile
import glob
import shutil
from datetime import datetime

def backup_dir(path, dest_dir=None, max_backups=0):
    path = os.path.abspath(path)
    if not os.path.isdir(path):
        return None, f"not a directory: {path}"

    if dest_dir is None:
        dest_dir = os.path.dirname(path) or "."
    dest_dir = os.path.abspath(dest_dir)
    os.makedirs(dest_dir, exist_ok=True)

    # 生成带时间戳的文件名
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    base = os.path.basename(path.rstrip("/"))
    backup_name = f"{base}_{timestamp}.tar.gz"
    backup_path = os.path.join(dest_dir, backup_name)

    # 创建 tar.gz
    file_count = 0
    try:
        with tarfile.open(backup_path, "w:gz") as tar:
            for root, dirs, files in os.walk(path):
                for fname in files:
                    fp = os.path.join(root, fname)
                    tar.add(fp, arcname=os.path.relpath(fp, os.path.dirname(path)))
                    file_count += 1
        size_bytes = os.path.getsize(backup_path)
    except Exception as e:
        if os.path.exists(backup_path):
            os.remove(backup_path)
        return None, str(e)

    # 清理旧备份
    pruned = []
    if max_backups > 0:
        pattern = os.path.join(dest_dir, f"{base}_*.tar.gz")
        backups = sorted(glob.glob(pattern), key=os.path.getmtime, reverse=True)
        for old in backups[max_backups:]:
            os.remove(old)
            pruned.append(old)

    return {
        "backup_path": backup_path,
        "size_bytes": size_bytes,
        "files_count": file_count,
        "pruned": pruned
    }, None

def main():
    try:
        params = json.load(sys.stdin)
    except Exception:
        params = {}

    path = params.get("path", "").strip()
    if not path:
        print(json.dumps({"status": "error", "error": "path is required"}))
        sys.exit(1)

    dest_dir = params.get("dest")
    max_backups = int(params.get("max_backups", 0))

    result, err = backup_dir(path, dest_dir, max_backups)
    if err:
        print(json.dumps({"status": "error", "error": err}))
        sys.exit(1)

    print(json.dumps({"status": "ok", **result}))

if __name__ == "__main__":
    main()
