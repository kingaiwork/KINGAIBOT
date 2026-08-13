import { appDataDir } from '@tauri-apps/api/path';
import { platform } from '@tauri-apps/plugin-os';
import { Client, Stronghold } from '@tauri-apps/plugin-stronghold';
import {
  isPermissionGranted,
  requestPermission,
  sendNotification,
} from '@tauri-apps/plugin-notification';
import type { SecureProfile } from './types';

const VAULT_CLIENT = 'kingaibot-control-center';
const PROFILE_KEY = 'server-profile';

async function openVault(password: string): Promise<{ stronghold: Stronghold; client: Client }> {
  if (password.length < 8) {
    throw new Error('Vault password must be at least 8 characters.');
  }
  const base = await appDataDir();
  const sep = base.endsWith('/') || base.endsWith('\\') ? '' : '/';
  const vaultPath = `${base}${sep}kingaibot-control-center.hold`;
  const stronghold = await Stronghold.load(vaultPath, password);
  let client: Client;
  try {
    client = await stronghold.loadClient(VAULT_CLIENT);
  } catch {
    client = await stronghold.createClient(VAULT_CLIENT);
  }
  return { stronghold, client };
}

export async function saveSecureProfile(profile: SecureProfile, password: string): Promise<void> {
  const { stronghold, client } = await openVault(password);
  const store = client.getStore();
  const bytes = Array.from(new TextEncoder().encode(JSON.stringify(profile)));
  await store.insert(PROFILE_KEY, bytes);
  await stronghold.save();
}

export async function loadSecureProfile(password: string): Promise<SecureProfile> {
  const { client } = await openVault(password);
  const store = client.getStore();
  const data = await store.get(PROFILE_KEY);
  if (!data || data.length === 0) {
    throw new Error('No encrypted KINGAIBOT profile is stored on this device.');
  }
  const raw = new TextDecoder().decode(new Uint8Array(data));
  const parsed = JSON.parse(raw) as Partial<SecureProfile>;
  if (!parsed.serverUrl || !parsed.token) {
    throw new Error('Stored profile is incomplete.');
  }
  return { serverUrl: parsed.serverUrl, token: parsed.token };
}

export async function requireApprovalConfirmation(): Promise<void> {
  const os = await platform();
  if (os !== 'android' && os !== 'ios') {
    return;
  }
  const { checkStatus, authenticate } = await import('@tauri-apps/plugin-biometric');
  const status = await checkStatus();
  if (!status.isAvailable) {
    throw new Error('Biometric authentication is not available on this device.');
  }
  await authenticate('Approve this exact KINGAIBOT action', {
    allowDeviceCredential: true,
    title: 'KINGAIBOT approval',
    subtitle: 'Confirm before the server executes this action',
    confirmationRequired: true,
  });
}

export async function notify(title: string, body: string): Promise<void> {
  let granted = await isPermissionGranted();
  if (!granted) {
    granted = (await requestPermission()) === 'granted';
  }
  if (granted) {
    sendNotification({ title, body });
  }
}
