import { useEffect, useState } from 'react';
import { createDevicePairing, listDevices, revokeDevice } from './api';
import type { DeviceRecord, PairingResult } from './types';

const defaultScopes = [
  'tasks:read',
  'tasks:create',
  'tasks:cancel',
  'approvals:read',
  'approvals:decide',
  'evolution:read',
];

const shortId = (value: string) => (value.length > 18 ? `${value.slice(0, 9)}…${value.slice(-6)}` : value);
const formatTime = (value?: string) => value ? new Date(value).toLocaleString() : '—';

interface Props {
  busy: boolean;
  setBusy: (value: boolean) => void;
  setError: (value: string) => void;
}

export default function DevicesPanel({ busy, setBusy, setError }: Props) {
  const [devices, setDevices] = useState<DeviceRecord[]>([]);
  const [pairing, setPairing] = useState<PairingResult | null>(null);
  const [adminAvailable, setAdminAvailable] = useState(true);

  const refresh = async () => {
    setBusy(true);
    setError('');
    try {
      const result = await listDevices();
      setDevices(result.devices ?? []);
      setAdminAvailable(true);
    } catch (err) {
      setAdminAvailable(false);
      setError('Device administration requires an Admin Token session. Device-scoped sessions remain intentionally unable to enumerate or provision other devices.');
    } finally {
      setBusy(false);
    }
  };

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const createPairing = async () => {
    setBusy(true);
    setError('');
    try {
      const result = await createDevicePairing(defaultScopes, 300);
      setPairing(result);
      setAdminAvailable(true);
    } catch (err) {
      setAdminAvailable(false);
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const revoke = async (device: DeviceRecord) => {
    if (device.revoked_at) return;
    setBusy(true);
    setError('');
    try {
      await revokeDevice(device.id);
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  };

  return (
    <section className="card-list">
      <div className="notice-panel device-admin-head">
        <div>
          <strong>Trusted devices</strong>
          <p>Provision short-lived pairings and revoke individual clients. This administrative surface never appears as a permission grant by itself; the server still requires the Admin Token.</p>
        </div>
        <button className="primary" disabled={busy || !adminAvailable} onClick={() => void createPairing()}>Create 5-minute pairing</button>
      </div>

      {!adminAvailable && (
        <div className="notice-panel restricted-panel">
          <strong>Device-scoped session</strong>
          <p>This client can use only its assigned scopes. Reconnect with the Admin Token on an administrator workstation to provision or revoke devices.</p>
        </div>
      )}

      {pairing && (
        <article className="data-card pairing-card">
          <div className="card-head">
            <div><span className="pill pending">one time</span><h3>Pairing ready</h3></div>
            <span>{formatTime(pairing.expires_at)}</span>
          </div>
          <p className="muted">Enter these values on the new Windows, macOS or Android client before expiration. The secret is shown for this session only.</p>
          <div className="pairing-secret-grid">
            <label>Pairing ID<code>{pairing.pairing_id}</code></label>
            <label>One-time secret<code>{pairing.pairing_secret}</code></label>
          </div>
          <div className="meta-row">{pairing.scopes.map((scope) => <span key={scope}>{scope}</span>)}</div>
          <button className="secondary compact" onClick={() => setPairing(null)}>Hide secret</button>
        </article>
      )}

      {adminAvailable && devices.length === 0 && (
        <div className="empty"><div>◇</div><h3>No paired devices</h3><p>Create a one-time pairing to enroll the first Control Center.</p></div>
      )}

      {devices.map((device) => (
        <article className="data-card" key={device.id}>
          <div className="card-head">
            <div><span className={`pill ${device.revoked_at ? 'denied' : 'approved'}`}>{device.revoked_at ? 'revoked' : 'trusted'}</span><h3>{device.name}</h3></div>
            <code>{shortId(device.id)}</code>
          </div>
          <p className="muted">{device.platform}</p>
          <div className="meta-row"><span>Enrolled {formatTime(device.created_at)}</span>{device.revoked_at && <span>Revoked {formatTime(device.revoked_at)}</span>}</div>
          <div className="meta-row">{device.scopes.map((scope) => <span key={scope}>{scope}</span>)}</div>
          {!device.revoked_at && <button className="danger-link" disabled={busy} onClick={() => void revoke(device)}>Revoke this device</button>}
        </article>
      ))}
    </section>
  );
}
