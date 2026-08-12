# 双仓使用说明（仅 baize_real）

## Remote

| 名称 | URL | 用途 |
|------|-----|------|
| `real` | https://github.com/rebornace/baize_real.git | 日常开发（含 `docs/superpowers/**`） |
| `public` | https://github.com/rebornace/baize.git | 开源发布（干净树） |

```powershell
git remote -v
```

## 日常

```powershell
git add .
git commit -m "type(scope): 中文说明"
git push -u real main
```

## 发布到开源仓

在仓库根目录执行（生成临时干净目录并提示 push 命令）：

```powershell
.\scripts\export-public.ps1
```

脚本会复制可发布文件到 `_public_export/`（排除 `docs/superpowers`、`.git`、工具残留），你在该目录单独 init/push 到 `public`，或按脚本输出的说明操作。
