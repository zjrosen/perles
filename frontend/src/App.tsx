import { useState, useEffect } from 'react'
import type { Session } from './types'
import SessionLoader from './components/SessionLoader'
import SessionViewer from './components/SessionViewer'
import './App.css'

function App() {
  const [session, setSession] = useState<Session | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [initialLoading, setInitialLoading] = useState(() => {
    // Check if we have a path in URL - if so, we'll be loading
    const params = new URLSearchParams(window.location.search)
    return !!params.get('path')
  })

  // Load session from URL param on mount
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const pathFromUrl = params.get('path')
    if (pathFromUrl) {
      handleLoad(pathFromUrl).finally(() => setInitialLoading(false))
    }
  }, [])

  const handleLoad = async (path: string) => {
    setError(null)
    try {
      const res = await fetch('/api/load-session', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path })
      })
      
      const data = await res.json()
      
      if (!res.ok) {
        throw new Error(data.error || 'Failed to load session')
      }
      
      setSession(data)
      
      // Update URL with session path
      const url = new URL(window.location.href)
      url.searchParams.set('path', path)
      window.history.replaceState({}, '', url.toString())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    }
  }

  const handleClear = () => {
    setSession(null)
    setError(null)
    
    // Clear URL params
    const url = new URL(window.location.href)
    url.searchParams.delete('path')
    url.searchParams.delete('tab')
    window.history.replaceState({}, '', url.toString())
  }

  return (
    <div className="app">
      <header className="app-header">
        <h1>🔮 Perles Session Viewer</h1>
        {session && (
          <div className="header-right">
            <code className="session-id-display">{session.metadata?.session_id}</code>
            <button className="refresh-btn" onClick={() => handleLoad(session.path)} title="Refresh data">
              ↻
            </button>
            <button className="clear-btn" onClick={handleClear}>
              Load Different Session
            </button>
          </div>
        )}
      </header>
      
      <main className="app-main">
        {initialLoading ? (
          <div className="initial-loading">Loading session...</div>
        ) : !session ? (
          <SessionLoader onLoad={handleLoad} error={error} />
        ) : (
          <SessionViewer session={session} onRefresh={() => handleLoad(session.path)} />
        )}
      </main>
    </div>
  )
}

export default App
