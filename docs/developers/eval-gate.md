# 评测回归门禁 CLI

> 适用：`baize eval-gate`。直接读取配置的存储，不需要启动 HTTP 服务。

## 用法

```bash
baize eval-gate -config configs/config.yaml
```

| 参数 | 说明 |
|------|------|
| `-config` | 配置文件路径，用于确定存储驱动与路径（默认 `configs/config.yaml`） |
| `-min` | 最低通过率，0–1；不传时读取设置页保存的阈值，仍没有则 0.9 |
| `-limit` | 最多评分多少个已确认用例（默认 500，上限 500） |
| `-json` | 输出 JSON 结果 |
| `-report` | 额外写一份 Markdown 报告 |

退出码：

- `0`：通过率达标。
- `1`：未达标，或没有任何已确认用例。
- 其他：配置/存储/参数错误。

评分是确定性的，不调用模型、不产生费用。

## 示例

```bash
# 终端查看
baize eval-gate -config configs/config.yaml

# CI：失败时保留报告
baize eval-gate -config configs/config.yaml -min 0.9 -report eval-gate.md
```

GitHub Actions 片段（自托管 runner，能访问部署使用的数据库）：

```yaml
- name: Eval regression gate
  run: |
    go build -o baize ./cmd/baize
    ./baize eval-gate -config configs/config.yaml -min 0.9 -report eval-gate.md
- name: Upload gate report
  if: always()
  uses: actions/upload-artifact@v4
  with:
    name: eval-gate
    path: eval-gate.md
```

## 口径

- 只统计 `confirmed` 用例；草稿不参与。
- 与设置页“运行回归”使用同一套评分代码与阈值语义。
- 空集不算通过。
- 单次最多 500 个用例；超出部分不会自动纳入。
