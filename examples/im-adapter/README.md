# im-adapter（参考适配器）

baize 进程外 webhook 渠道的最小参考实现（Go 标准库，无第三方依赖），演示
适配器↔baize 的 JSON-over-HTTP + HMAC 协议。任意语言可照此实现。

## 运行

1. baize 配置一个 webhook 渠道实例（config.yaml）：

   ```yaml
   channels:
     - name: demo
       type: webhook
       enabled: true
       config:
         source: demo
         secret: dev-secret
         outbound_url: http://127.0.0.1:9100/outbound
         assignee: u-admin
   ```

2. 启动适配器：

   ```bash
   go run ./examples/im-adapter -baize http://127.0.0.1:8080/v0/channels/demo/inbound -secret dev-secret
   ```

3. 模拟 IM 用户发来消息：

   ```bash
   curl "http://127.0.0.1:9100/send?peer=alice&text=你好"
   ```

   baize 建 run、引擎产出后，适配器日志会打印 `[baize->adapter] ...` 出站消息。

## 协议要点

- 入站（适配器→baize）：`POST {baize}/v0/channels/{name}/inbound`，头
  `X-Baize-Channel-Timestamp`（unix 秒）、`X-Baize-Channel-Signature`
  （`v1=` + HMAC-SHA256(secret, timestamp+"."+body)）。
- 出站（baize→适配器）：`POST outbound_url`，同样的签名头；适配器必须验签。
- 适配器验签时必须同时校验时间戳新鲜度（±300s）：出站方向 baize 是发送方，
  重放防护只能靠适配器自己，时间戳解析失败或偏差超过 300 秒的请求应拒绝（401）。
- 时间窗 ±300s；重放/过期请求被拒。
