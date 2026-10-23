# Baize mobile shell (Capacitor)

Cross-platform **client** for the operator console. The Go Runtime stays on a server; this package wraps [`web/chat`](../web/chat) in a WebView.

Full guide (Capacitor shell **and** on-device trimmed orchestration): [docs/developers/mobile.md](../docs/developers/mobile.md) · [EN](../docs/developers/mobile.en.md)

```powershell
copy .env.example .env
# Recommended: BAIZE_SERVER_URL=http://<lan-or-public-host>:8080
npm install
npm run sync
npm run add:android
npm run open:android
```

Native `android/` / `ios/` trees are gitignored — generate them with `add:android` / `add:ios`.

**This package** does not embed `baize serve`. On-device trimmed orchestration lives in `mobileagent/` + `mobile/rt-shell*` (see the mobile developer docs).
