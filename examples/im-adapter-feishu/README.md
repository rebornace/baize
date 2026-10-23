# im-adapter-feishu（飞书参考适配器）

在 [`../im-adapter`](../im-adapter) 协议之上，演示如何把 **飞书事件订阅** 归一成
baize webhook inbound。本样例用标准库，不引入飞书 SDK；生产可换官方 SDK，
但 **HMAC 出站验签 + 时间窗** 必须保留。

## baize 配置

```yaml
channels:
  - name: feishu
    type: webhook
    enabled: true
    config:
      source: feishu
      secret: dev-secret
      outbound_url: http://127.0.0.1:9101/outbound
      assignee: u-admin
```

## 运行

```bash
go run ./examples/im-adapter-feishu \
  -baize http://127.0.0.1:8080/v0/channels/feishu/inbound \
  -secret dev-secret \
  -addr :9101
```

模拟飞书文本消息（简化 JSON，非完整飞书 schema）：

```bash
curl -X POST http://127.0.0.1:9101/feishu/event \
  -H 'Content-Type: application/json' \
  -d '{"schema":"2.0","header":{"event_type":"im.message.receive_v1"},"event":{"sender":{"sender_id":{"open_id":"ou_alice"}},"message":{"message_type":"text","content":"{\"text\":\"你好\"}"}}}'
```

适配器会映射为 baize inbound：`peer.id=ou_alice`，`text=你好`。

## 映射约定（可按租户改）

| 飞书字段 | baize inbound |
|----------|---------------|
| `event.sender.sender_id.open_id`（或缺则 `user_id`） | `peer.id` / `peer.name` |
| `event.message.content`（text JSON 的 `text`） | `text` |
| `header.event_id`（若有） | `idempotency_key` |

URL 校验 / 加密推送等飞书平台细节未实现：接到真实租户时在适配器侧补齐，
再调用同一 `forwardInbound` 即可。
