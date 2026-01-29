import { useState } from 'react'
import type { Command } from '../types'
import './CommandsPanel.css'

interface Props {
  commands: Command[]
}

export default function CommandsPanel({ commands }: Props) {
  const [expandedCommands, setExpandedCommands] = useState<Set<number>>(new Set())
  const [typeFilters, setTypeFilters] = useState<Set<string>>(new Set())
  const [sourceFilters, setSourceFilters] = useState<Set<string>>(new Set())

  const toggleTypeFilter = (type: string) => {
    setTypeFilters(prev => {
      const next = new Set(prev)
      if (next.has(type)) {
        next.delete(type)
      } else {
        next.add(type)
      }
      return next
    })
  }

  const toggleSourceFilter = (source: string) => {
    setSourceFilters(prev => {
      const next = new Set(prev)
      if (next.has(source)) {
        next.delete(source)
      } else {
        next.add(source)
      }
      return next
    })
  }

  const formatTime = (ts: string) => {
    const date = new Date(ts)
    return date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit', second: '2-digit' })
  }

  const getCommandColor = (type: string): string => {
    const colors: Record<string, string> = {
      'spawn_process': 'var(--accent-purple)',
      'send_to_process': 'var(--accent-blue)',
      'deliver_process_queued': 'var(--accent-blue)',
      'process_turn_complete': 'var(--accent-green)',
      'assign_task': 'var(--accent-yellow)',
      'mark_task_complete': 'var(--accent-green)',
      'mark_task_failed': 'var(--accent-red)',
      'report_complete': 'var(--accent-green)',
      'notify_user': 'var(--accent-orange)',
    }
    return colors[type] || 'var(--text-muted)'
  }

  const getSourceBadge = (source: string): string => {
    switch (source) {
      case 'mcp_tool': return 'MCP'
      case 'internal': return 'INT'
      case 'callback': return 'CB'
      default: return source.toUpperCase().slice(0, 3)
    }
  }

  const toggleExpand = (idx: number) => {
    const newExpanded = new Set(expandedCommands)
    if (newExpanded.has(idx)) {
      newExpanded.delete(idx)
    } else {
      newExpanded.add(idx)
    }
    setExpandedCommands(newExpanded)
  }

  const commandTypes = [...new Set(commands.map(c => c.command_type))]
  const sources = [...new Set(commands.map(c => c.source))]
  const filteredCommands = commands.filter(c => {
    if (typeFilters.size > 0 && !typeFilters.has(c.command_type)) return false
    if (sourceFilters.size > 0 && !sourceFilters.has(c.source)) return false
    return true
  })

  return (
    <div className="commands-panel">
      <div className="commands-header">
        <div className="filter-rows">
          <div className="filter-buttons">
            <span className="filter-label">Type:</span>
            <button
              className={`filter-btn ${typeFilters.size === 0 ? 'active' : ''}`}
              onClick={() => setTypeFilters(new Set())}
            >
              All
            </button>
            {commandTypes.map(type => (
              <button
                key={type}
                className={`filter-btn ${typeFilters.has(type) ? 'active' : ''}`}
                onClick={() => toggleTypeFilter(type)}
              >
                {type}
              </button>
            ))}
          </div>
          <div className="filter-buttons">
            <span className="filter-label">Source:</span>
            <button
              className={`filter-btn ${sourceFilters.size === 0 ? 'active' : ''}`}
              onClick={() => setSourceFilters(new Set())}
            >
              All
            </button>
            {sources.map(source => (
              <button
                key={source}
                className={`filter-btn ${sourceFilters.has(source) ? 'active' : ''}`}
                onClick={() => toggleSourceFilter(source)}
              >
                {getSourceBadge(source)}
              </button>
            ))}
          </div>
        </div>
      </div>
      <div className="commands-list">
        {filteredCommands.map((cmd, idx) => {
          const processId = cmd.payload?.ProcessID as string || cmd.result_data?.ProcessID as string
          const taskId = cmd.payload?.TaskID as string || cmd.result_data?.TaskID as string
          const workerId = cmd.payload?.WorkerID as string || cmd.result_data?.WorkerID as string
          
          return (
            <div key={idx} className={`command-item ${cmd.success ? '' : 'failed'}`}>
              <button className="command-header" onClick={() => toggleExpand(idx)}>
                <span className="command-expand">
                  {expandedCommands.has(idx) ? '▼' : '▶'}
                </span>
                <span className="command-type" style={{ color: getCommandColor(cmd.command_type) }}>
                  {cmd.command_type}
                </span>
                <span className={`command-source source-${cmd.source}`}>
                  {getSourceBadge(cmd.source)}
                </span>
                {processId && (
                  <span className="command-process">{processId}</span>
                )}
                {workerId && (
                  <span className="command-worker">{workerId}</span>
                )}
                {taskId && (
                  <span className="command-task">{taskId}</span>
                )}
                <span className="command-duration">{cmd.duration_ms}ms</span>
                <span className="command-time">{formatTime(cmd.timestamp)}</span>
                {!cmd.success && <span className="command-failed-badge">FAILED</span>}
              </button>
              {expandedCommands.has(idx) && (
                <div className={`command-detail ${!cmd.result_data ? 'single-column' : ''}`}>
                  <div className="detail-section">
                    <h4>Payload</h4>
                    <pre>{JSON.stringify(cmd.payload, null, 2)}</pre>
                  </div>
                  {cmd.result_data && (
                    <div className="detail-section">
                      <h4>Result</h4>
                      <pre>{JSON.stringify(cmd.result_data, null, 2)}</pre>
                    </div>
                  )}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
