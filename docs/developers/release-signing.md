# 发布签名与密钥交接

> 适用：`scripts/sign-release.ps1`、本地 cosign 密钥、`SHA256SUMS.txt` 与 OCI 归档。
> 不在范围：Keyless / OIDC、KMS、镜像仓库签名。

---

## 1. 生成密钥

```powershell
cosign generate-key-pair
# 生成 cosign.key（加密私钥）与 cosign.pub
```

推荐用脚本生成到独立目录，并自动写指纹交接文件：

```powershell
$env:COSIGN_PASSWORD = '<password>'
.\scripts\rotate-release-key.ps1 -OutDir D:\offline\baize-key-2026-11
# 生成 cosign.key / cosign.pub / KEY-HANDOFF.txt
.\scripts\rotate-release-key.ps1 -OutDir D:\offline\baize-key-2026-11 -CheckOnly `
  -ExpectedFingerprint <sha256>
```

- 已有 `cosign.key` 时脚本拒绝覆盖；轮换请用新目录。
- `-CheckOnly` 只校验指纹，不写文件。
- 脚本不会打印私钥；`KEY-HANDOFF.txt` 只含公钥名、指纹、创建时间与后续步骤。

- 私钥密码单独保管，不与私钥同目录、不同仓库、不入 CI 日志。
- **私钥永不入库**：不提交、不作为 Release 资产、不发聊天/邮件。
- 公钥可公开：随 Release 上传 `cosign.pub`，指纹写入发布记录。

## 2. 指纹

脚本会输出公钥指纹（DER SPKI 的 SHA256），并写入：

```text
<OutDir>/cosign-key-fingerprint.txt
public_key=cosign.pub
sha256=<64 hex>
signed_at=<UTC RFC3339>
```

签名前可传入期望值，不匹配立即失败（不会开始签名）：

```powershell
.\scripts\sign-release.ps1 `
  -Key D:\offline\cosign.key `
  -ArtifactsDir dist\release\v0.36.0 `
  -OciArchive R:\out\baize-oci.tar `
  -OutDir dist\release\v0.36.0\signatures `
  -ExpectedFingerprint 83b3949348897f156e768e691dbf84a177da797b3738cc387a9564261c7aaebd
```

指纹计算口径：去掉 PEM 头尾与空白，Base64 解码为 DER，再做 SHA256。同一公钥换行/CRLF 不影响。

## 3. 离线保管

- 私钥放离线介质或受控密码库；至少两人可恢复，但签名权限最小化。
- 备份加密私钥与密码分开存放；记录保管人、位置、创建时间、指纹。
- 发布机只临时挂载私钥；签名后清理工作目录与 shell 历史中的密码。
- 不把私钥放进 `dist/`、Git、对象存储公开桶或 GitHub Actions 日志。

## 4. 交接

1. 旧保管人记录当前公钥指纹与最近一次签名 Release。
2. 通过加密通道（1Password/Bitwarden、age、GPG）传输私钥；密码另走通道。
3. 新保管人用公钥重算指纹，必须与发布记录一致。
4. 用新保管环境对一个**临时 dummy 文件**跑一次 `sign-release.ps1` + `verify-blob`。
5. 更新保管人列表；旧保管人删除本地副本（保留离线备份策略由团队决定）。
6. 下一次发布记录写入新指纹与签名人。

## 5. 轮换

1. `cosign generate-key-pair` 生成新对。
2. 用新私钥签下一版；公钥作为新 Release 资产上传。
3. 发布说明写清新指纹；旧公钥继续保留，用于验证旧 Release。
4. **不要**用新密钥重签旧产物；旧签名对应旧公钥。
5. 公告轮换日期与新旧指纹。

## 6. 吊销 / 私钥泄露

1. 立即停止使用该私钥，移出可信列表。
2. 发布安全公告，列出该指纹签过的版本与时间范围。
3. 能重发则用新密钥重签并发布；不能重发则标记相关产物为“签名不可信”。
4. 旧公钥保留用于取证，但验证结果需附泄露说明。
5. 轮换后检查 Release 资产、文档、CI secret 中是否还有旧私钥。

## 7. 验证

```bash
cosign verify-blob --key cosign.pub \
  --bundle SHA256SUMS.txt.sigstore.json --offline SHA256SUMS.txt

cosign verify-blob --key cosign.pub \
  --bundle baize_vX.Y.Z_oci.tar.sigstore.json --offline baize_vX.Y.Z_oci.tar
```

- `Verified OK` 只证明“该公钥签过该文件”；仍需确认公钥指纹与发布记录一致。
- 指纹不一致时，不要把 `cosign.pub` 当作可信根。

## 8. CI 边界

- 首选：发布者在离线/受控机器手动签名，CI 只构建与扫描。
- 如必须进 CI：使用受保护 Environment + 人工审批 + secret 注入；禁止回显私钥；签名后立即清理 runner。
- 本轮不做 Keyless / OIDC / KMS；需要时另开设计，不复用本流程的私钥。

## 9. 发布检查清单

- [ ] `-ExpectedFingerprint` 与发布记录一致
- [ ] `cosign-key-fingerprint.txt` 随 Release 上传
- [ ] `cosign.pub` 随 Release 上传
- [ ] `SHA256SUMS.txt` 与 OCI 归档 `Verified OK`
- [ ] 旧公钥仍可验证旧 Release
- [ ] 私钥未进入仓库、日志、构建产物
