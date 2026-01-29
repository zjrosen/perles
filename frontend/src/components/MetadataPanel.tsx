import type { Session, SessionMetadata } from '../types'
import './MetadataPanel.css'

interface Props {
  metadata: SessionMetadata
  session: Session
}

export default function MetadataPanel({ metadata, session }: Props) {
  const formatTime = (ts: string) => {
    return new Date(ts).toLocaleString()
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'running': return 'var(--accent-green)'
      case 'completed': return 'var(--accent-blue)'
      case 'failed': return 'var(--accent-red)'
      default: return 'var(--text-secondary)'
    }
  }

  const fabricStats = {
    channels: session.fabric.filter(e => e.event.type === 'channel.created').length,
    messages: session.fabric.filter(e => e.event.type === 'message.posted').length,
    replies: session.fabric.filter(e => e.event.type === 'reply.posted').length,
    acks: session.fabric.filter(e => e.event.type === 'acked').length,
  }

  const commands = session.commands || []
  const commandTypes = [
    { type: 'spawn_process', label: 'Spawn', color: 'var(--accent-purple)' },
    { type: 'assign_task', label: 'Assign Task', color: 'var(--accent-yellow)' },
    { type: 'send_to_process', label: 'Send', color: 'var(--accent-blue)' },
    { type: 'deliver_process_queued', label: 'Deliver', color: 'var(--accent-blue)' },
    { type: 'process_turn_complete', label: 'Turn Done', color: 'var(--accent-green)' },
    { type: 'mark_task_complete', label: 'Task Done', color: 'var(--accent-green)' },
    { type: 'mark_task_failed', label: 'Task Failed', color: 'var(--accent-red)' },
    { type: 'report_complete', label: 'Report', color: 'var(--accent-green)' },
    { type: 'notify_user', label: 'Notify', color: 'var(--accent-orange)' },
  ]
  const commandCounts = commandTypes.map(ct => ({
    ...ct,
    count: commands.filter(c => c.command_type === ct.type).length,
  })).filter(ct => ct.count > 0)
  const commandStats = {
    total: commands.length,
    successRate: commands.length > 0 ? Math.round((commands.filter(c => c.success).length / commands.length) * 100) : 100,
  }

  return (
    <div className="metadata-panel">
      <div className="overview-columns">
        {/* Left Column */}
        <div className="overview-left">
          <section className="meta-section">
            <h2>Session Info</h2>
            <div className="meta-grid">
              <div className="meta-item">
                <label>Session ID</label>
                <code>{metadata.session_id}</code>
              </div>
              <div className="meta-item">
                <label>Status</label>
                <span className="status-badge" style={{ background: getStatusColor(metadata.status) }}>
                  {metadata.status}
                </span>
              </div>
              <div className="meta-item">
                <label>Application</label>
                <span>{metadata.application_name}</span>
              </div>
              <div className="meta-item">
                <label>Started</label>
                <span>{formatTime(metadata.start_time)}</span>
              </div>
              <div className="meta-item">
                <label>Work Directory</label>
                <code>{metadata.work_dir}</code>
              </div>
              <div className="meta-item">
                <label>Resumable</label>
                <span>{metadata.resumable ? '✅ Yes' : '❌ No'}</span>
              </div>
            </div>
          </section>

          <section className="meta-section">
            <h2>Token Usage</h2>
            <div className="meta-grid">
              <div className="meta-item">
                <label>Input Tokens</label>
                <span className="token-count">{metadata.token_usage.total_input_tokens.toLocaleString()}</span>
              </div>
              <div className="meta-item">
                <label>Output Tokens</label>
                <span className="token-count">{metadata.token_usage.total_output_tokens.toLocaleString()}</span>
              </div>
              <div className="meta-item">
                <label>Total Cost</label>
                <span className="cost">${metadata.token_usage.total_cost_usd.toFixed(4)}</span>
              </div>
            </div>
          </section>

          <section className="meta-section">
            <h2>Files</h2>
            <div className="meta-grid">
              <div className="meta-item">
                <label>Session Directory</label>
                <code>{metadata.session_dir}</code>
              </div>
              <div className="meta-item">
                <label>MCP Requests</label>
                <span>{session.mcpRequests.length} logged</span>
              </div>
              <div className="meta-item">
                <label>Coordinator Messages</label>
                <span>{session.coordinator.messages.length}</span>
              </div>
            </div>
          </section>
        </div>

        {/* Right Column */}
        <div className="overview-right">
          <section className="meta-section">
            <h2>Fabric Activity</h2>
            <div className="stats-grid">
              <div className="stat-card">
                <span className="stat-value">{fabricStats.channels}</span>
                <span className="stat-label">Channels</span>
              </div>
              <div className="stat-card">
                <span className="stat-value">{fabricStats.messages}</span>
                <span className="stat-label">Messages</span>
              </div>
              <div className="stat-card">
                <span className="stat-value">{fabricStats.replies}</span>
                <span className="stat-label">Replies</span>
              </div>
              <div className="stat-card">
                <span className="stat-value">{fabricStats.acks}</span>
                <span className="stat-label">Acks</span>
              </div>
            </div>
          </section>

          <section className="meta-section">
            <h2>Commands Activity</h2>
            <div className="stats-grid commands-stats">
              {commandCounts.map(ct => (
                <div key={ct.type} className="stat-card">
                  <span className="stat-value" style={{ color: ct.color }}>{ct.count}</span>
                  <span className="stat-label">{ct.label}</span>
                </div>
              ))}
            </div>
            <div className="command-summary">
              <span>{commandStats.total} total commands</span>
              <span className="success-rate" style={{ color: commandStats.successRate === 100 ? 'var(--accent-green)' : 'var(--accent-yellow)' }}>
                {commandStats.successRate}% success
              </span>
            </div>
          </section>

          <section className="meta-section">
            <h2>Workers ({metadata.workers.length})</h2>
            <div className="workers-list">
              {metadata.workers.map(worker => (
                <div key={worker.id} className="worker-card">
                  <div className="worker-header">
                    <span className="worker-id">{worker.id}</span>
                    <span className="worker-time">{formatTime(worker.spawned_at)}</span>
                  </div>
                  <div className="worker-details">
                    <code>{worker.headless_session_ref}</code>
                  </div>
                </div>
              ))}
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}
