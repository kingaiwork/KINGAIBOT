import { invoke } from '@tauri-apps/api/core';
import type {
  ApprovalListResponse,
  DeviceListResponse,
  EvolutionListResponse,
  PairDeviceResult,
  PairingResult,
  ServerSummary,
  Task,
  TaskListResponse,
} from './types';

export async function connectServer(serverUrl: string, token: string): Promise<ServerSummary> {
  return invoke<ServerSummary>('connect_server', {
    args: { serverUrl, token },
  });
}

export async function pairDevice(
  serverUrl: string,
  pairingId: string,
  pairingSecret: string,
  deviceName: string,
  platform?: string,
): Promise<PairDeviceResult> {
  return invoke<PairDeviceResult>('pair_device', {
    args: { serverUrl, pairingId, pairingSecret, deviceName, platform },
  });
}

export async function disconnectServer(): Promise<void> {
  await invoke('disconnect_server');
}

export async function serverStatus(): Promise<ServerSummary> {
  return invoke<ServerSummary>('server_status');
}

export async function listTasks(): Promise<TaskListResponse> {
  return invoke<TaskListResponse>('list_tasks');
}

export async function createTask(input: string): Promise<Task> {
  return invoke<Task>('create_task', { input });
}

export async function cancelTask(id: string): Promise<void> {
  await invoke('cancel_task', { id });
}

export async function listApprovals(): Promise<ApprovalListResponse> {
  return invoke<ApprovalListResponse>('list_approvals');
}

export async function decideApproval(id: string, status: 'approved' | 'denied'): Promise<void> {
  await invoke('decide_approval', { id, status });
}

export async function listEvolution(): Promise<EvolutionListResponse> {
  return invoke<EvolutionListResponse>('list_evolution');
}

export async function createDevicePairing(
  scopes: string[],
  expiresInSeconds = 300,
): Promise<PairingResult> {
  return invoke<PairingResult>('create_device_pairing', {
    args: { scopes, expiresInSeconds },
  });
}

export async function listDevices(): Promise<DeviceListResponse> {
  return invoke<DeviceListResponse>('list_devices');
}

export async function revokeDevice(id: string): Promise<void> {
  await invoke('revoke_device', { id });
}
