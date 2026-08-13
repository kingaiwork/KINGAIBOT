#!/usr/bin/env python3
from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"{label} patch target not found")
    return text.replace(old, new, 1)


p = Path("clients/control-center/src/App.tsx")
s = p.read_text()

s = replace_once(
    s,
    "import './styles.css';\n\ntype View = 'overview' | 'tasks' | 'approvals' | 'evolution';",
    "import PairDevicePanel from './PairDevicePanel';\nimport DevicesPanel from './DevicesPanel';\nimport './styles.css';\n\ntype View = 'overview' | 'tasks' | 'approvals' | 'evolution' | 'devices';",
    "App imports/View",
)

anchor = """  const onConnect = async (event: FormEvent) => {
    event.preventDefault();
    if (token.trim().length < 32) {
      setError('Admin token must contain at least 32 characters.');
      return;
    }
"""
replacement = """  const onConnect = async (event: FormEvent) => {
    event.preventDefault();
    if (token.trim().length < 32) {
      setError('Access token must contain at least 32 characters.');
      return;
    }
"""
s = replace_once(s, anchor, replacement, "access-token copy")

connect_form = """          <form onSubmit={onConnect} className=\"connect-form\">
            <label>
              Server
              <input value={serverUrl} onChange={(e) => setServerUrl(e.target.value)} autoCapitalize=\"none\" autoCorrect=\"off\" />
            </label>
            <label>
              Admin token
              <input type=\"password\" value={token} onChange={(e) => setToken(e.target.value)} autoComplete=\"off\" />
            </label>
            <label>
              Local vault password
              <input type=\"password\" value={vaultPassword} onChange={(e) => setVaultPassword(e.target.value)} autoComplete=\"off\" placeholder=\"Needed only to save/load an encrypted profile\" />
            </label>
            <label className=\"check-row\">
              <input type=\"checkbox\" checked={persistProfile} onChange={(e) => setPersistProfile(e.target.checked)} />
              <span>Save this server profile in the encrypted device vault</span>
            </label>
            {error && <div className=\"error-banner\">{error}</div>}
            <div className=\"connect-actions\">
              <button className=\"primary\" disabled={busy}>{busy ? 'Connecting…' : 'Connect securely'}</button>
              <button type=\"button\" className=\"secondary\" disabled={busy || vaultPassword.length < 8} onClick={onLoadProfile}>Load encrypted profile</button>
            </div>
          </form>
          <p className=\"security-note\">Remote servers require HTTPS. Plain HTTP is accepted only for loopback/local runtime mode.</p>
"""
connect_replacement = """          <PairDevicePanel
            initialServerUrl={serverUrl}
            busy={busy}
            setBusy={setBusy}
            setError={setError}
            onConnected={(result, url) => {
              setServerUrl(url);
              setSummary(result);
              setConnected(true);
            }}
          />
          <div className=\"connection-divider\"><span>or use an existing credential</span></div>
          <form onSubmit={onConnect} className=\"connect-form existing-credential-form\">
            <div className=\"mode-heading\">
              <h2>Existing access token</h2>
              <p>Use a previously paired Device Token for routine operation, or an Admin Token only on an administrator workstation.</p>
            </div>
            <label>
              Server
              <input value={serverUrl} onChange={(e) => setServerUrl(e.target.value)} autoCapitalize=\"none\" autoCorrect=\"off\" />
            </label>
            <label>
              Access token
              <input type=\"password\" value={token} onChange={(e) => setToken(e.target.value)} autoComplete=\"off\" />
            </label>
            <label>
              Local vault password
              <input type=\"password\" value={vaultPassword} onChange={(e) => setVaultPassword(e.target.value)} autoComplete=\"off\" placeholder=\"Needed only to save/load an encrypted profile\" />
            </label>
            <label className=\"check-row\">
              <input type=\"checkbox\" checked={persistProfile} onChange={(e) => setPersistProfile(e.target.checked)} />
              <span>Save this credential in the encrypted device vault</span>
            </label>
            {error && <div className=\"error-banner\">{error}</div>}
            <div className=\"connect-actions\">
              <button className=\"primary\" disabled={busy}>{busy ? 'Connecting…' : 'Connect securely'}</button>
              <button type=\"button\" className=\"secondary\" disabled={busy || vaultPassword.length < 8} onClick={onLoadProfile}>Load encrypted profile</button>
            </div>
          </form>
          <p className=\"security-note\">Remote servers require HTTPS. Plain HTTP is accepted only for loopback/local runtime mode.</p>
"""
s = replace_once(s, connect_form, connect_replacement, "connection surface")

nav_anchor = """            <NavButton current={view} id=\"approvals\" label=\"Approvals\" badge={pendingApprovals.length} onClick={setView} />
            <NavButton current={view} id=\"evolution\" label=\"Evolution\" badge={proposals.length} onClick={setView} />
"""
nav_replacement = nav_anchor + """            <NavButton current={view} id=\"devices\" label=\"Devices\" onClick={setView} />
"""
s = replace_once(s, nav_anchor, nav_replacement, "desktop devices nav")

view_anchor = """        {view === 'evolution' && (
          <section className=\"card-list\">
"""
# Devices panel is deliberately separate from refresh(): a scoped device token
# is expected to receive 403 from admin-only device-management endpoints.
insert = """        {view === 'devices' && (
          <DevicesPanel busy={busy} setBusy={setBusy} setError={setError} />
        )}

"""
if view_anchor not in s:
    raise SystemExit("view insertion target not found")
s = s.replace(view_anchor, insert + view_anchor, 1)

s = replace_once(
    s,
    "return { overview: 'Control Center', tasks: 'Durable Tasks', approvals: 'Action Approvals', evolution: 'Evolution Proposals' }[view];",
    "return { overview: 'Control Center', tasks: 'Durable Tasks', approvals: 'Action Approvals', evolution: 'Evolution Proposals', devices: 'Trusted Devices' }[view];",
    "title map",
)
p.write_text(s)

p = Path("clients/control-center/src/styles.css")
s = p.read_text()
if ".mode-heading" not in s:
    s += r'''

/* V1.3 pairing and device-identity surfaces */
.mode-heading { display: grid; gap: 6px; margin-bottom: 3px; }
.mode-heading h2 { margin: 0; font-size: 19px; letter-spacing: -.025em; }
.mode-heading p { margin: 0; color: var(--muted); font-size: 13px; font-weight: 500; line-height: 1.55; }
.mode-badge { justify-self: start; display: inline-flex; border-radius: 999px; padding: 5px 9px; background: #e1efe5; color: #356b4d; font-size: 10px; font-weight: 800; letter-spacing: .06em; text-transform: uppercase; }
.pairing-form { margin-top: 28px; padding: 20px; border: 1px solid var(--line); border-radius: 20px; background: rgba(255,255,255,.35); }
.existing-credential-form { padding: 4px 0 0; }
.connection-divider { display: flex; align-items: center; gap: 12px; margin: 22px 0 3px; color: #91887e; font-size: 11px; text-transform: uppercase; letter-spacing: .08em; }
.connection-divider::before, .connection-divider::after { content: ''; height: 1px; flex: 1; background: var(--line); }
.two-field-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.device-admin-head { display: flex; align-items: center; justify-content: space-between; gap: 18px; }
.restricted-panel { border-style: dashed; }
.pairing-card { border-color: rgba(125,93,32,.18); }
.pairing-secret-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin: 16px 0; }
.pairing-secret-grid label { min-width: 0; }
.pairing-secret-grid code { display: block; margin-top: 5px; padding: 11px; border-radius: 12px; background: #f4f0e9; color: #4c443c; overflow-wrap: anywhere; user-select: all; font: 12px/1.55 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }

@media (max-width: 720px) {
  .two-field-grid, .pairing-secret-grid { grid-template-columns: 1fr; }
  .device-admin-head { align-items: stretch; flex-direction: column; }
  .pairing-form { padding: 16px; }
}

@media (prefers-color-scheme: dark) {
  .pairing-form { background: rgba(255,255,255,.025); }
  .pairing-secret-grid code { background: #211e1b; color: #ded6cc; }
}
'''
p.write_text(s)

p = Path("clients/control-center/README.md")
s = p.read_text()
s = s.replace(
    "- Connect to a local or remote KINGAIBOT Runtime.\n",
    "- Pair a new Windows/macOS/Android device with a short-lived one-time server pairing.\n- Connect to a local or remote KINGAIBOT Runtime with a revocable Device Token or administrator credential.\n",
    1,
)
s = s.replace(
    "The current V1.3 client can use the Runtime admin token. This is an interim bootstrap model. V1.4 replaces routine remote use with short-lived QR pairing and revocable device-scoped credentials.",
    "V1.3 now supports one-time server pairing and revocable device-scoped credentials. Admin Token login remains available for trusted administrator workstations and device provisioning. QR-camera enrollment is the next UX layer; the security primitive is already the one-time Pairing ID + secret flow.",
    1,
)
s = s.replace(
    "V1.3 暂时允许使用 Runtime Admin Token 完成早期联调。这只是过渡方案；V1.4 将改为 QR 短时配对 + 可撤销设备级凭据，让手机和桌面不再长期持有服务器管理员身份。",
    "V1.3 已加入一次性服务器配对与可撤销设备级凭据。Admin Token 登录仅保留给可信管理员工作站和设备管理；下一步 QR 扫码只是把现有 Pairing ID + Secret 流程做成更自然的交互，不再把管理员密钥复制到手机或普通桌面端。",
    1,
)
p.write_text(s)
