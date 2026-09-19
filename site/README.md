# 白泽官网（GitHub Pages）

静态站点源码。推送到开源仓 `main` 后，由 [`.github/workflows/pages.yml`](../.github/workflows/pages.yml) 部署。

- 中文：[`index.html`](./index.html)
- English：[`en/index.html`](./en/index.html)

首次启用：开源仓 **Settings → Pages → Build and deployment → Source** 选 **GitHub Actions**（须管理员操作一次；`GITHUB_TOKEN` 无法代开）。  
站点地址：https://rebornace.github.io/baize/

说明：`pages.yml` 仅在 `rebornace/baize` 执行部署；私有仓同步同文件会跳过，避免 “Get Pages site failed”。
