# Busabase OpenAPI GET-only filter

裁剪 [Busabase 公开 OpenAPI](https://busabase.com/api/v1/openapi.json) 为仅 `GET` 的 JSON，供 EXT-SoR **examples 配方**使用（非产品绑定）。接入走白泽既有「设置 → 业务系统」。

## 再生成

```powershell
Invoke-WebRequest https://busabase.com/api/v1/openapi.json -OutFile $env:TEMP\busabase-openapi.json
node scripts/ext-sor-busabase/filter_readonly_openapi.mjs $env:TEMP\busabase-openapi.json examples/ext-sor-busabase/openapi.readonly.json
```

输出会在 `info.description` 前缀写入 `Baize EXT-SoR readonly filter; source version <上游 info.version>`。

## 测试

```powershell
node --test scripts/ext-sor-busabase/filter_readonly_openapi.test.mjs
```
