# Release signing and key handoff

> Applies to `scripts/sign-release.ps1`, local cosign keys, `SHA256SUMS.txt` and the OCI archive.
> Out of scope: Keyless / OIDC, KMS, registry signing.

---

## 1. Generate a key

```powershell
cosign generate-key-pair
# writes cosign.key (encrypted private key) and cosign.pub
```

Prefer the script when creating a new directory and a handoff record:

```powershell
$env:COSIGN_PASSWORD = '<password>'
.\scripts\rotate-release-key.ps1 -OutDir D:\offline\baize-key-2026-11
# writes cosign.key / cosign.pub / KEY-HANDOFF.txt
.\scripts\rotate-release-key.ps1 -OutDir D:\offline\baize-key-2026-11 -CheckOnly `
  -ExpectedFingerprint <sha256>
```

- It refuses to overwrite an existing `cosign.key`; rotate into a new directory.
- `-CheckOnly` verifies the fingerprint without writing files.
- It never prints the private key; `KEY-HANDOFF.txt` contains only the public key name, fingerprint, timestamp and next steps.

- Keep the private-key password separate from the key and repository.
- **The private key never enters the repo**: no commit, no Release asset, no chat/email.
- The public key is publishable: upload `cosign.pub` with each Release and record its fingerprint.

## 2. Fingerprint

The script prints the public-key fingerprint (SHA256 of DER SPKI) and writes:

```text
<OutDir>/cosign-key-fingerprint.txt
public_key=cosign.pub
sha256=<64 hex>
signed_at=<UTC RFC3339>
```

Pass the expected value before signing; a mismatch fails before any signature:

```powershell
.\scripts\sign-release.ps1 `
  -Key D:\offline\cosign.key `
  -ArtifactsDir dist\release\v0.36.0 `
  -OciArchive R:\out\baize-oci.tar `
  -OutDir dist\release\v0.36.0\signatures `
  -ExpectedFingerprint 83b3949348897f156e768e691dbf84a177da797b3738cc387a9564261c7aaebd
```

Definition: strip PEM headers/footers and whitespace, Base64-decode to DER, SHA256. CRLF vs LF does not change it.

## 3. Offline custody

- Keep the private key on offline media or in a controlled vault; at least two people can recover it, but signing rights are minimized.
- Store encrypted backups separately from the password; record custodian, location, creation time and fingerprint.
- Mount the key only for the signing step; clean the working directory and shell history afterwards.
- Never put the private key in `dist/`, Git, a public bucket, or GitHub Actions logs.

## 4. Handoff

1. The old custodian records the current fingerprint and the last Release signed with it.
2. Transfer the private key over an encrypted channel (1Password/Bitwarden, age, GPG); send the password over a different channel.
3. The new custodian recomputes the fingerprint from the public key and checks it against the release record.
4. Sign a temporary dummy file with the new environment and run `verify-blob`.
5. Update the custodian list; the old custodian deletes local copies (offline backup policy stays a team decision).
6. The next release record lists the new fingerprint and signer.

## 5. Rotation

1. Run `cosign generate-key-pair`.
2. Sign the next version with the new key; upload the new public key as a Release asset.
3. Announce the new fingerprint; keep the old public key for old Releases.
4. **Do not** re-sign old artifacts with the new key; old bundles belong to the old key.
5. Publish the rotation date and both fingerprints.

## 6. Revocation / key compromise

1. Stop using the key and remove it from the trusted set.
2. Publish an advisory listing the fingerprint and the affected versions/time window.
3. Re-release with a new key when possible; otherwise mark the affected assets as untrusted.
4. Keep the old public key for forensics, but qualify verification results with the compromise note.
5. Check Release assets, docs and CI secrets for the old private key.

## 7. Verify

```bash
cosign verify-blob --key cosign.pub \
  --bundle SHA256SUMS.txt.sigstore.json --offline SHA256SUMS.txt

cosign verify-blob --key cosign.pub \
  --bundle baize_vX.Y.Z_oci.tar.sigstore.json --offline baize_vX.Y.Z_oci.tar
```

- `Verified OK` only proves "this public key signed this file"; still compare the fingerprint with the release record.
- If the fingerprint does not match, do not treat `cosign.pub` as a trusted root.

## 8. CI boundary

- Preferred: the release owner signs manually on an offline or controlled machine; CI only builds and scans.
- If CI must sign: protected Environment + manual approval + secret injection; never echo the private key; clean the runner afterwards.
- This slice does not implement Keyless / OIDC / KMS; design that separately rather than reusing this key flow.

## 9. Release checklist

- [ ] `-ExpectedFingerprint` matches the release record
- [ ] `cosign-key-fingerprint.txt` uploaded with the Release
- [ ] `cosign.pub` uploaded with the Release
- [ ] `SHA256SUMS.txt` and OCI archive are `Verified OK`
- [ ] Old public keys still verify old Releases
- [ ] Private key is absent from repo, logs and build outputs
