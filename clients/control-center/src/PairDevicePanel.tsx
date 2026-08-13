import { FormEvent, useState } from 'react';
import { pairDevice } from './api';
import { saveSecureProfile } from './security';
import type { ServerSummary } from './types';

interface Props {
  initialServerUrl: string;
  busy: boolean;
  setBusy: (value: boolean) => void;
  setError: (value: string) => void;
  onConnected: (summary: ServerSummary, serverUrl: string) => void;
}

export default function PairDevicePanel({
  initialServerUrl,
  busy,
  setBusy,
  setError,
  onConnected,
}: Props) {
  const [serverUrl, setServerUrl] = useState(initialServerUrl);
  const [pairingId, setPairingId] = useState('');
  const [pairingSecret, setPairingSecret] = useState('');
  const [deviceName, setDeviceName] = useState('My device');
  const [saveProfile, setSaveProfile] = useState(true);
  const [vaultPassword, setVaultPassword] = useState('');

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!serverUrl.trim() || !pairingId.trim() || pairingSecret.trim().length < 32) {
      setError('Server, Pairing ID and the complete one-time pairing secret are required.');
      return;
    }
    if (!deviceName.trim()) {
      setError('Give this device a recognizable name.');
      return;
    }
    if (saveProfile && vaultPassword.length < 8) {
      setError('Use a local vault password of at least 8 characters to save the device credential.');
      return;
    }

    setBusy(true);
    setError('');
    try {
      const result = await pairDevice(
        serverUrl.trim(),
        pairingId.trim(),
        pairingSecret.trim(),
        deviceName.trim(),
      );
      if (saveProfile) {
        await saveSecureProfile(
          { serverUrl: serverUrl.trim(), token: result.deviceToken },
          vaultPassword,
        );
      }
      setPairingSecret('');
      setPairingId('');
      onConnected(result.summary, serverUrl.trim());
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="connect-form pairing-form">
      <div className="mode-heading">
        <span className="mode-badge">Recommended</span>
        <h2>Pair this device</h2>
        <p>Use the short-lived Pairing ID and one-time secret created by a trusted KINGAIBOT administrator. No Admin Token is copied to this device.</p>
      </div>
      <label>
        Server
        <input value={serverUrl} onChange={(e) => setServerUrl(e.target.value)} autoCapitalize="none" autoCorrect="off" />
      </label>
      <div className="two-field-grid">
        <label>
          Pairing ID
          <input value={pairingId} onChange={(e) => setPairingId(e.target.value)} autoCapitalize="none" autoCorrect="off" placeholder="pair_…" />
        </label>
        <label>
          Device name
          <input value={deviceName} onChange={(e) => setDeviceName(e.target.value)} maxLength={80} placeholder="Jack's Android" />
        </label>
      </div>
      <label>
        One-time pairing secret
        <input type="password" value={pairingSecret} onChange={(e) => setPairingSecret(e.target.value)} autoComplete="off" />
      </label>
      <label>
        Local vault password
        <input type="password" value={vaultPassword} onChange={(e) => setVaultPassword(e.target.value)} autoComplete="off" placeholder="Encrypt the device credential on this device" />
      </label>
      <label className="check-row">
        <input type="checkbox" checked={saveProfile} onChange={(e) => setSaveProfile(e.target.checked)} />
        <span>Store the new device credential in the encrypted local vault</span>
      </label>
      <button className="primary" disabled={busy}>{busy ? 'Pairing…' : 'Pair securely'}</button>
      <p className="security-note">Pairings expire quickly and can be used only once. The resulting credential can be revoked without rotating the server Admin Token.</p>
    </form>
  );
}
