import { FormEvent, useCallback, useEffect, useRef, useState } from 'react';
import {
  cancelTask,
  connectServer,
  createTask,
  decideApproval,
  disconnectServer,
  listApprovals,
  listEvolution,
  listTasks,
  serverStatus,
} from './api';
import { loadSecureProfile, notify, requireApprovalConfirmation, saveSecureProfile } from './security';
import type { Approval, EvolutionProposal, ServerSummary, Task } from './types';
import PairDevicePanel from './PairDevicePanel';
import DevicesPanel from './DevicesPanel';
import './styles.css';

type View = 'overview' | 'tasks' | 'approvals' | 'evolution' | 'devices';

const formatTime = (value?: string) => {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
};

const shortId = (value: string) => (value.length > 18 ? `${value.slice(0, 9)}…${value.slice(-6)}` : value);

export default function App() {
  const [view, setView] = useState<View>('overview');
  const [serverUrl, setServerUrl] = useState('http://127.0.0.1:18888');
  const [token, setToken] = useState('');
  const [vaultPassword, setVaultPassword] = useState('');
  const [persistProfile, setPersistProfile] = useState(false);
  const [connected, setConnected] = useState(false);
  const [summary, setSummary] = useState<ServerSummary | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [approvals, setApprovals] = useState<Approval[]>([]);
  const [proposals, setProposals] = useState<EvolutionProposal[]>([]);
  const [taskInput, setTaskInput] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const previousPending = useRef(0);

  const refresh = useCallback(async (quiet = false) => {
    if (!connected) return;
    try {
      if (!quiet) setBusy(true);
      const [status, taskData, approvalData, evolutionData] = await Promise.all([
        serverStatus(),
        listTasks(),
        listApprovals(),
        listEvolution(),
      ]);
      setSummary(status);
      setTasks(taskData.tasks ?? []);
      setApprovals(approvalData.approvals ?? []);
      setProposals(evolutionData.proposals ?? []);
      const pending = (approvalData.approvals ?? []).filter((item) => item.status === 'pending').length;
      if (pending > previousPending.current && previousPending.current >= 0) {
        void notify('KINGAIBOT approval required', `${pending} action${pending === 1 ? '' : 's'} waiting for review.`);
      }
      previousPending.current = pending;
      setError('');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      if (!quiet) setBusy(false);
    }
  }, [connected]);

  useEffect(() => {
    if (!connected) return;
    void refresh();
    const timer = window.setInterval(() => void refresh(true), 5000);
    return () => window.clearInterval(timer);
  }, [connected, refresh]);

  const performConnect = async (url: string, secret: string, saveAfterConnect: boolean) => {
    setBusy(true);
    setError('');
    try {
      const result = await connectServer(url.trim(), secret.trim());
      if (saveAfterConnect) {
        await saveSecureProfile({ serverUrl: url.trim(), token: secret.trim() }, vaultPassword);
      }
      setSummary(result);
      setConnected(true);
      setToken('');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const onConnect = async (event: FormEvent) => {
    event.preventDefault();
    if (token.trim().length < 32) {
      setError('Access token must contain at least 32 characters.');
      return;
    }
    if (persistProfile && vaultPassword.length < 8) {
      setError('Enter a local vault password of at least 8 characters before saving this profile.');
      return;
    }
    await performConnect(serverUrl, token, persistProfile);
  };

  const onLoadProfile = async () => {
    setBusy(true);
    setError('');
    try {
      const profile = await loadSecureProfile(vaultPassword);
      setServerUrl(profile.serverUrl);
      await performConnect(profile.serverUrl, profile.token, false);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  };

  const onDisconnect = async () => {
    await disconnectServer();
    setConnected(false);
    setSummary(null);
    setTasks([]);
    setApprovals([]);
    setProposals([]);
    previousPending.current = 0;
  };

  const onCreateTask = async (event: FormEvent) => {
    event.preventDefault();
    const input = taskInput.trim();
    if (!input) return;
    setBusy(true);
    try {
      await createTask(input);
      setTaskInput('');
      await refresh(true);
      setView('tasks');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const onCancelTask = async (id: string) => {
    setBusy(true);
    try {
      await cancelTask(id);
      await refresh(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const onDecision = async (item: Approval, status: 'approved' | 'denied') => {
    setBusy(true);
    setError('');
    try {
      if (status === 'approved') {
        await requireApprovalConfirmation();
      }
      await decideApproval(item.id, status);
      await refresh(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  if (!connected) {
    return (
      <main className="connect-shell">
        <section className="connect-card">
          <div className="brand-mark">K</div>
          <p className="eyebrow">KING AI · Execution Layer</p>
          <h1>KINGAIBOT</h1>
          <p className="lede">A quiet control surface for durable agents, approvals and real-world execution.</p>
          <PairDevicePanel
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
          <div className="connection-divider"><span>or use an existing credential</span></div>
          <form onSubmit={onConnect} className="connect-form existing-credential-form">
            <div className="mode-heading">
              <h2>Existing access token</h2>
              <p>Use a previously paired Device Token for routine operation, or an Admin Token only on an administrator workstation.</p>
            </div>
            <label>
              Server
              <input value={serverUrl} onChange={(e) => setServerUrl(e.target.value)} autoCapitalize="none" autoCorrect="off" />
            </label>
            <label>
              Access token
              <input type="password" value={token} onChange={(e) => setToken(e.target.value)} autoComplete="off" />
            </label>
            <label>
              Local vault password
              <input type="password" value={vaultPassword} onChange={(e) => setVaultPassword(e.target.value)} autoComplete="off" placeholder="Needed only to save/load an encrypted profile" />
            </label>
            <label className="check-row">
              <input type="checkbox" checked={persistProfile} onChange={(e) => setPersistProfile(e.target.checked)} />
              <span>Save this credential in the encrypted device vault</span>
            </label>
            {error && <div className="error-banner">{error}</div>}
            <div className="connect-actions">
              <button className="primary" disabled={busy}>{busy ? 'Connecting…' : 'Connect securely'}</button>
              <button type="button" className="secondary" disabled={busy || vaultPassword.length < 8} onClick={onLoadProfile}>Load encrypted profile</button>
            </div>
          </form>
          <p className="security-note">Remote servers require HTTPS. Plain HTTP is accepted only for loopback/local runtime mode.</p>
        </section>
      </main>
    );
  }

  const pendingApprovals = approvals.filter((item) => item.status === 'pending');
  const activeTasks = tasks.filter((item) => ['queued', 'running', 'waiting_approval'].includes(item.status));

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div>
          <div className="mini-brand"><span>K</span><strong>KINGAIBOT</strong></div>
          <nav>
            <NavButton current={view} id="overview" label="Overview" onClick={setView} />
            <NavButton current={view} id="tasks" label="Tasks" badge={activeTasks.length} onClick={setView} />
            <NavButton current={view} id="approvals" label="Approvals" badge={pendingApprovals.length} onClick={setView} />
            <NavButton current={view} id="evolution" label="Evolution" badge={proposals.length} onClick={setView} />
            <NavButton current={view} id="devices" label="Devices" onClick={setView} />
          </nav>
        </div>
        <div className="server-foot">
          <span className={`status-dot ${summary?.ready ? 'online' : 'warn'}`} />
          <div><strong>{summary?.name ?? 'KINGAIBOT'}</strong><small>{summary?.version ?? '—'}</small></div>
          <button className="text-button" onClick={onDisconnect}>Disconnect</button>
        </div>
      </aside>

      <main className="workspace">
        <header className="topbar">
          <div><p className="eyebrow">{summary?.baseUrl}</p><h2>{titleFor(view)}</h2></div>
          <button className="secondary compact" disabled={busy} onClick={() => void refresh()}>{busy ? 'Working…' : 'Refresh'}</button>
        </header>
        {error && <div className="error-banner workspace-error">{error}</div>}

        {view === 'overview' && (
          <section className="stack">
            <div className="hero-panel">
              <div><p className="eyebrow">Terminal intelligence, under control</p><h3>{summary?.ready ? 'Runtime ready.' : 'Runtime connected, readiness needs attention.'}</h3><p>Tasks remain durable on the server. This client is only the trusted human control surface.</p></div>
              <div className="pulse-orb" aria-hidden="true" />
            </div>
            <div className="metrics-grid">
              <Metric label="Active tasks" value={activeTasks.length} note={`${tasks.length} total visible`} />
              <Metric label="Waiting approval" value={pendingApprovals.length} note="Exact-action approval" />
              <Metric label="Evolution proposals" value={proposals.length} note="Proposal-only mode" />
              <Metric label="Server" value={summary?.ready ? 'Ready' : 'Check'} note={summary?.version ?? '—'} />
            </div>
            <form className="task-composer" onSubmit={onCreateTask}>
              <label htmlFor="task">New task</label>
              <textarea id="task" value={taskInput} onChange={(e) => setTaskInput(e.target.value)} placeholder="Describe the outcome you want KINGAIBOT to achieve…" rows={4} />
              <div><button className="primary" disabled={busy || !taskInput.trim()}>Send to runtime</button></div>
            </form>
          </section>
        )}

        {view === 'tasks' && (
          <section className="card-list">
            {tasks.length === 0 && <Empty title="No tasks yet" text="Create a task from Overview and it will appear here." />}
            {tasks.map((item) => (
              <article className="data-card" key={item.id}>
                <div className="card-head"><div><span className={`pill ${item.status}`}>{item.status.replace('_', ' ')}</span><h3>{item.input}</h3></div><code>{shortId(item.id)}</code></div>
                {item.output && <pre className="result-box">{item.output}</pre>}
                {item.error && <p className="error-text">{item.error}</p>}
                <div className="meta-row"><span>Attempts {item.attempts}</span><span>Updated {formatTime(item.updated_at)}</span></div>
                {['queued', 'running', 'waiting_approval'].includes(item.status) && <button className="danger-link" disabled={busy} onClick={() => void onCancelTask(item.id)}>Cancel task</button>}
              </article>
            ))}
          </section>
        )}

        {view === 'approvals' && (
          <section className="card-list">
            {approvals.length === 0 && <Empty title="No approvals" text="Actions configured as ASK will appear here before execution." />}
            {approvals.map((item) => (
              <article className={`data-card approval-card ${item.status}`} key={item.id}>
                <div className="card-head"><div><span className={`pill ${item.status}`}>{item.status}</span><h3>{item.tool}</h3></div><code>{shortId(item.arguments_hash)}</code></div>
                <p className="muted">Task {shortId(item.task_id)} · capability {item.capability}</p>
                <details open={item.status === 'pending'}><summary>Exact arguments</summary><pre className="result-box">{JSON.stringify(item.arguments ?? {}, null, 2)}</pre></details>
                <div className="meta-row"><span>Created {formatTime(item.created_at)}</span><span>{item.execution_state || 'not executed'}</span></div>
                {item.status === 'pending' && <div className="decision-row"><button className="primary" disabled={busy} onClick={() => void onDecision(item, 'approved')}>Approve exact action</button><button className="secondary" disabled={busy} onClick={() => void onDecision(item, 'denied')}>Deny</button></div>}
              </article>
            ))}
          </section>
        )}

        {view === 'devices' && (
          <DevicesPanel busy={busy} setBusy={setBusy} setError={setError} />
        )}

        {view === 'evolution' && (
          <section className="card-list">
            <div className="notice-panel"><strong>Controlled evolution</strong><p>These are improvement proposals, not self-authorized code changes. Production activation still requires testing, review and the release pipeline.</p></div>
            {proposals.length === 0 && <Empty title="No proposals" text="Runtime-generated improvement proposals will be shown here." />}
            {proposals.map((item) => (
              <article className="data-card" key={item.id}>
                <div className="card-head"><div><span className="pill proposal">{item.risk || 'review'}</span><h3>{item.title}</h3></div><code>{shortId(item.id)}</code></div>
                <p>{item.rationale}</p>
                {item.evidence && <details><summary>Evidence</summary><pre className="result-box">{JSON.stringify(item.evidence, null, 2)}</pre></details>}
                <div className="meta-row"><span>{item.kind}</span><span>{item.status}</span><span>{formatTime(item.created_at)}</span></div>
              </article>
            ))}
          </section>
        )}
      </main>

      <nav className="mobile-nav">
        <NavButton current={view} id="overview" label="Home" onClick={setView} />
        <NavButton current={view} id="tasks" label="Tasks" badge={activeTasks.length} onClick={setView} />
        <NavButton current={view} id="approvals" label="Approve" badge={pendingApprovals.length} onClick={setView} />
        <NavButton current={view} id="evolution" label="Evolve" onClick={setView} />
      </nav>
    </div>
  );
}

function titleFor(view: View) {
  return { overview: 'Control Center', tasks: 'Durable Tasks', approvals: 'Action Approvals', evolution: 'Evolution Proposals', devices: 'Trusted Devices' }[view];
}

function NavButton({ current, id, label, badge, onClick }: { current: View; id: View; label: string; badge?: number; onClick: (view: View) => void }) {
  return <button className={current === id ? 'nav-active' : ''} onClick={() => onClick(id)}><span>{label}</span>{badge !== undefined && badge > 0 && <b>{badge}</b>}</button>;
}

function Metric({ label, value, note }: { label: string; value: string | number; note: string }) {
  return <article className="metric-card"><span>{label}</span><strong>{value}</strong><small>{note}</small></article>;
}

function Empty({ title, text }: { title: string; text: string }) {
  return <div className="empty"><div>✦</div><h3>{title}</h3><p>{text}</p></div>;
}
