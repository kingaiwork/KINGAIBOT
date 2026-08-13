import { invoke } from '@tauri-apps/api/core';
import type {
  ApprovalListResponse,
  EvolutionListResponse,
  ServerSummary,
  Task,
  TaskListResponse,
} from './types';

export async function connectServer(serverUrl: string, token: string): Promise<ServerSummary> {
  return invoke<ServerSummary>('connect_server', {
    args: { serverUrl, token },
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
