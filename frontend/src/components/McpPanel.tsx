import { useState } from 'react'
import type { McpRequest } from '../types'
import './McpPanel.css'

interface Props {
  requests: McpRequest[]
}

export default function McpPanel({ requests }: Props) {
  const [expandedIds, setExpandedIds] = useState<Set<number>>(new Set())
  const [toolFilters, setToolFilters] = useState<Set<string>>(new Set())
  const [workerFilters, setWorkerFilters] = useState<Set<string>>(new Set())

  const toggleToolFilter = (name: string) => {
    setToolFilters(prev => {
      const next = new Set(prev)
      if (next.has(name)) {
        next.delete(name)
      } else {
        next.add(name)
      }
      return next
    })
  }

  const toggleWorkerFilter = (id: string) => {
    setWorkerFilters(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const toggleExpand = (index: number) => {
    setExpandedIds(prev => {
      const next = new Set(prev)
      if (next.has(index)) {
        next.delete(index)
      } else {
        next.add(index)
      }
      return next
    })
  }

  const formatTime = (ts: string) => {
    return new Date(ts).toLocaleTimeString()
  }

  const formatDuration = (ns: number) => {
    if (ns < 1000000) {
      return `${(ns / 1000).toFixed(1)}µs`
    }
    if (ns < 1000000000) {
      return `${(ns / 1000000).toFixed(1)}ms`
    }
    return `${(ns / 1000000000).toFixed(2)}s`
  }

  const decodeBase64 = (str: string): unknown => {
    try {
      return JSON.parse(atob(str))
    } catch {
      return str
    }
  }

  const toolNames = [...new Set(requests.map(r => r.tool_name))]
  const workerIds = ['coordinator', ...new Set(requests.map(r => r.worker_id).filter(Boolean))] as string[]
  const filteredRequests = requests.filter(r => {
    if (toolFilters.size > 0 && !toolFilters.has(r.tool_name)) return false
    if (workerFilters.size > 0) {
      const effectiveWorkerId = r.worker_id || 'coordinator'
      if (!workerFilters.has(effectiveWorkerId)) return false
    }
    return true
  })

  return (
    <div className="mcp-panel">
      <div className="mcp-header">
        <h2>MCP Tool Calls ({filteredRequests.length})</h2>
        <div className="filter-rows">
          <div className="filter-buttons">
            <span className="filter-label">Tool:</span>
            <button
              className={`filter-btn ${toolFilters.size === 0 ? 'active' : ''}`}
              onClick={() => setToolFilters(new Set())}
            >
              All
            </button>
            {toolNames.map(name => (
              <button
                key={name}
                className={`filter-btn ${toolFilters.has(name) ? 'active' : ''}`}
                onClick={() => toggleToolFilter(name)}
              >
                {name}
              </button>
            ))}
          </div>
          {workerIds.length > 0 && (
            <div className="filter-buttons">
              <span className="filter-label">Worker:</span>
              <button
                className={`filter-btn ${workerFilters.size === 0 ? 'active' : ''}`}
                onClick={() => setWorkerFilters(new Set())}
              >
                All
              </button>
              {workerIds.map(id => (
                <button
                  key={id}
                  className={`filter-btn ${workerFilters.has(id) ? 'active' : ''}`}
                  onClick={() => toggleWorkerFilter(id)}
                >
                  {id}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="requests-list">
        {filteredRequests.map((req, index) => (
          <div 
            key={index}
            className={`request-item ${expandedIds.has(index) ? 'expanded' : ''}`}
            onClick={() => toggleExpand(index)}
          >
            <div className="request-header">
              <div className="request-info">
                <span className="tool-name">{req.tool_name}</span>
                {req.worker_id && (
                  <span className="worker-badge">{req.worker_id}</span>
                )}
              </div>
              <div className="request-meta">
                <span className="duration">{formatDuration(req.duration)}</span>
                <span className="time">{formatTime(req.timestamp)}</span>
              </div>
            </div>

            {expandedIds.has(index) && (
              <div className="request-details">
                <div className="detail-section">
                  <label>Request</label>
                  <pre>{JSON.stringify(decodeBase64(req.request_json), null, 2)}</pre>
                </div>
                <div className="detail-section">
                  <label>Response</label>
                  <pre>{JSON.stringify(decodeBase64(req.response_json), null, 2)}</pre>
                </div>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
