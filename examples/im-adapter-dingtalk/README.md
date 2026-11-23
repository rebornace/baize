# im-adapter-dingtalk（钉钉参考适配器）

在 [`../im-adapter`](../im-adapter) 协议之上，演示如何把 **钉钉机器人回调** 归一成
baize webhook inbound。标准库实现，无钉钉 SDK。

## baize 配置

```yaml
channels:
  - name: dingtalk
    type: webhook
    enabled: true
    config:
      source: dingtalk
      secret: dev-secret
      outbound_url: http://127.0.0.1:9102/outbound
      assignee: u-admin
```

## 运行

```bash
go run ./examples/im-adapter-dingtalk \
  -baize http://127.0.0.1:8080/v0/channels/dingtalk/inbound \
  -secret dev-secret \
  -addr :9102
```

模拟钉钉文本回调：

```bash
curl -X POST http://127.0.0.1:9102/dingtalk/callback \
  -H 'Content-Type: application/json' \
  -d '{"msgtype":"text","text":{"content":"你好"},"senderStaffId":"staff_alice","msgId":"msg-1"}'
```

## 映射约定

| 钉钉字段 | baize inbound |
|----------|---------------|
| `senderStaffId`（或缺则 `senderId`） | `peer.id` |
| `text.content`（`msgtype=text`） | `text` |
| `msgId` | `idempotency_key` |

出站：baize `POST /outbound` 验签后，生产侧应调用钉钉 OpenAPI 把
`text` 发回 `peer`；本样例只打日志。
