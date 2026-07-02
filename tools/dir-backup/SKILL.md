---
name: dir-backup
description: 备份目录到带时间戳的 tar.gz 归档文件，支持自动清理旧备份。当用户说"备份一下"、"回滚点"、"存档"时触发。
version: 1.0.0
xw_command: python3 scripts/backup.py
xw_params:
  path: 要备份的目录路径（必填）
  dest: 备份存放目录，默认与 path 同目录
  max_backups: 保留的最大备份数，默认不清理
xw_output:
  status: ok | error
  backup_path: 备份文件路径
  size_bytes: 备份文件大小（字节）
  files_count: 备份的文件数
  pruned: 被清理的旧备份列表（仅当 max_backups 生效时）
xw_examples:
  - description: 备份项目目录
    params: { path: /home/user/project }
  - description: 备份并保留最近 3 份
    params: { path: /home/user/project, max_backups: 3 }
---
