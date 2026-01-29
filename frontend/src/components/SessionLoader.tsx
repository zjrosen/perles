import { useState, useEffect } from 'react'
import './SessionLoader.css'

interface Props {
  onLoad: (path: string) => void
  error: string | null
}

interface SessionEntry {
  id: string
  path: string
  startTime: string | null
  status: string
  workerCount: number
  clientType: string
}

interface DateEntry {
  date: string
  sessions: SessionEntry[]
}

interface AppEntry {
  name: string
  dates: DateEntry[]
}

interface SessionsResponse {
  basePath: string
  apps: AppEntry[]
}

export default function SessionLoader({ onLoad, error }: Props) {
  const [sessions, setSessions] = useState<SessionsResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [fetchError, setFetchError] = useState<string | null>(null)
  const [expandedApps, setExpandedApps] = useState<Set<string>>(new Set())
  const [expandedDates, setExpandedDates] = useState<Set<string>>(new Set())

  useEffect(() => {
    fetchSessions()
  }, [])

  const fetchSessions = async () => {
    try {
      setFetchError(null)
      const res = await fetch('/api/sessions')
      
      if (!res.ok) {
        const errorData = await res.json().catch(() => ({}))
        throw new Error(errorData.error || `Server returned ${res.status}`)
      }
      
      const data = await res.json()
      
      if (!data.apps) {
        throw new Error('Invalid response: missing apps array')
      }
      
      setSessions(data)
      
      // Auto-expand first app if there's only one
      if (data.apps.length === 1) {
        setExpandedApps(new Set([data.apps[0].name]))
        // Auto-expand first date
        if (data.apps[0].dates.length > 0) {
          setExpandedDates(new Set([`${data.apps[0].name}/${data.apps[0].dates[0].date}`]))
        }
      }
    } catch (err) {
      console.error('Failed to fetch sessions:', err)
      setFetchError(err instanceof Error ? err.message : 'Failed to load sessions')
    } finally {
      setLoading(false)
    }
  }

  const toggleApp = (appName: string) => {
    setExpandedApps(prev => {
      const next = new Set(prev)
      if (next.has(appName)) {
        next.delete(appName)
      } else {
        next.add(appName)
      }
      return next
    })
  }

  const toggleDate = (key: string) => {
    setExpandedDates(prev => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  const formatTime = (ts: string | null) => {
    if (!ts) return ''
    const date = new Date(ts)
    return date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'running': return 'var(--accent-green)'
      case 'completed': return 'var(--accent-blue)'
      case 'failed': return 'var(--accent-red)'
      default: return 'var(--text-muted)'
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'running': return '🟢'
      case 'completed': return '✓'
      case 'failed': return '✗'
      default: return '○'
    }
  }

  if (loading) {
    return (
      <div className="session-loader">
        <div className="loader-card">
          <div className="loading-state">Loading sessions...</div>
        </div>
      </div>
    )
  }

  return (
    <div className="session-loader">
      <div className="loader-card explorer-card">
        <h2>🔮 Select Session</h2>
        <p className="loader-description">
          Browse sessions from <code>{sessions?.basePath}</code>
        </p>

        {(error || fetchError) && (
          <div className="error-message">
            ⚠️ {error || fetchError}
          </div>
        )}

        <div className="session-explorer">
          {sessions?.apps.length === 0 ? (
            <div className="empty-state">
              <p>No sessions found</p>
              <p className="empty-hint">Sessions are stored in ~/.perles/sessions/</p>
            </div>
          ) : (
            sessions?.apps.map(app => (
              <div key={app.name} className="app-group">
                <button 
                  className="app-header"
                  onClick={() => toggleApp(app.name)}
                >
                  <span className="expand-icon">
                    {expandedApps.has(app.name) ? '▼' : '▶'}
                  </span>
                  <span className="app-icon">📁</span>
                  <span className="app-name">{app.name}</span>
                  <span className="app-count">
                    {app.dates.reduce((sum, d) => sum + d.sessions.length, 0)} sessions
                  </span>
                </button>

                {expandedApps.has(app.name) && (
                  <div className="app-content">
                    {app.dates.map(dateEntry => {
                      const dateKey = `${app.name}/${dateEntry.date}`
                      return (
                        <div key={dateEntry.date} className="date-group">
                          <button
                            className="date-header"
                            onClick={() => toggleDate(dateKey)}
                          >
                            <span className="expand-icon">
                              {expandedDates.has(dateKey) ? '▼' : '▶'}
                            </span>
                            <span className="date-icon">📅</span>
                            <span className="date-name">{dateEntry.date}</span>
                            <span className="date-count">
                              {dateEntry.sessions.length}
                            </span>
                          </button>

                          {expandedDates.has(dateKey) && (
                            <div className="date-content">
                              {dateEntry.sessions.map(session => (
                                <button
                                  key={session.id}
                                  className="session-item"
                                  onClick={() => onLoad(session.path)}
                                >
                                  <span 
                                    className="session-status"
                                    style={{ color: getStatusColor(session.status) }}
                                    title={session.status}
                                  >
                                    {getStatusIcon(session.status)}
                                  </span>
                                  <div className="session-info">
                                    <span className="session-time">
                                      {formatTime(session.startTime)}
                                    </span>
                                    <span className="session-id">
                                      {session.id.slice(0, 8)}...
                                    </span>
                                  </div>
                                  <div className="session-meta">
                                    <span className="session-client">{session.clientType}</span>
                                    {session.workerCount > 0 && (
                                      <span className="session-workers">
                                        {session.workerCount} workers
                                      </span>
                                    )}
                                  </div>
                                </button>
                              ))}
                            </div>
                          )}
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
