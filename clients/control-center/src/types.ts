export type TaskStatus =
  | 'queued'
  | 'running'
  | 'waiting_approval'
  | 'completed'
  | 'failed'
  | 'canceled';

export interface ServerSummary {
  name: string;
  version: string;
  baseUrl: string;
  ready: boolean;
}

export interface Task {
  id: string;
  input: string;
  output?: string;
  status: TaskStatus;
  error?: string;
  attempts: number;
  created_at: string;
  updated_at: string;
  pending_approval?: string;
  metadata?: Record<string, unknown>;
}

export interface Approval {
  id: string;
  task_id: string;
  tool: string;
  capability: string;
  arguments?: unknown;
  arguments_hash: string;
  status: 'pending' | 'approved' | 'denied';
  execution_state?: string;
  result?: string;
  execution_error?: string;
  created_at: string;
  updated_at: string;
}

export interface EvolutionProposal {
  id: string;
  kind: string;
  title: string;
  rationale: string;
  evidence?: Record<string, unknown>;
  risk: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface TaskListResponse {
  tasks: Task[];
}

export interface ApprovalListResponse {
  approvals: Approval[];
}

export interface EvolutionListResponse {
  mode: string;
  enabled: boolean;
  proposals: EvolutionProposal[];
}

export interface SecureProfile {
  serverUrl: string;
  token: string;
}
