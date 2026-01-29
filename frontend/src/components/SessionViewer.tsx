import { useState } from 'react'
import type { Session } from '../types'
import MetadataPanel from './MetadataPanel'
import FabricPanel from './FabricPanel'
import AgentPanel from './AgentPanel'
import McpPanel from './McpPanel'
import CommandsPanel from './CommandsPanel'
import './SessionViewer.css'

interface Props {
  session: Session
  onRefresh: () => void
}

type Tab = 'overview' | 'fabric' | 'commands' | 'coordinator' | 'workers' | 'mcp'

const validTabs: Tab[] = ['overview', 'fabric', 'commands', 'coordinator', 'workers', 'mcp']

function getInitialTab(): Tab {
  const params = new URLSearchParams(window.location.search)
  const tabFromUrl = params.get('tab')
  if (tabFromUrl && validTabs.includes(tabFromUrl as Tab)) {
    return tabFromUrl as Tab
  }
  return 'overview'
}

export default function SessionViewer({ session, onRefresh }: Props) {
  const [activeTab, setActiveTab] = useState<Tab>(getInitialTab)
  const [selectedWorker, setSelectedWorker] = useState<string>(
    Object.keys(session.workers)[0] || ''
  )

  // Update URL when tab changes and refresh data
  const handleTabChange = (tab: Tab) => {
    setActiveTab(tab)
    const url = new URL(window.location.href)
    url.searchParams.set('tab', tab)
    window.history.replaceState({}, '', url.toString())
    onRefresh()
  }

  const tabs: { id: Tab; label: string; count?: number }[] = [
    { id: 'overview', label: 'Overview' },
    { id: 'fabric', label: 'Fabric', count: session.fabric.length },
    { id: 'commands', label: 'Commands', count: session.commands?.length || 0 },
    { id: 'coordinator', label: 'Coordinator', count: session.coordinator.messages.length },
    { id: 'workers', label: 'Workers', count: Object.keys(session.workers).length },
    { id: 'mcp', label: 'MCP Requests', count: session.mcpRequests.length },
  ]

  return (
    <div className="session-viewer">
      <nav className="viewer-tabs">
        {tabs.map(tab => (
          <button
            key={tab.id}
            className={`tab-btn ${activeTab === tab.id ? 'active' : ''}`}
            onClick={() => handleTabChange(tab.id)}
          >
            {tab.label}
            {tab.count !== undefined && (
              <span className="tab-count">{tab.count}</span>
            )}
          </button>
        ))}
      </nav>

      <div className="viewer-content">
        {activeTab === 'overview' && session.metadata && (
          <MetadataPanel metadata={session.metadata} session={session} />
        )}
        
        {activeTab === 'fabric' && (
          <FabricPanel events={session.fabric} />
        )}

        {activeTab === 'commands' && (
          <CommandsPanel commands={session.commands || []} />
        )}
        
        {activeTab === 'coordinator' && (
          <AgentPanel 
            name="Coordinator" 
            messages={session.coordinator.messages} 
          />
        )}
        
        {activeTab === 'workers' && (
          <div className="workers-view">
            <div className="worker-selector">
              {Object.keys(session.workers).map(workerId => (
                <button
                  key={workerId}
                  className={`worker-btn ${selectedWorker === workerId ? 'active' : ''}`}
                  onClick={() => setSelectedWorker(workerId)}
                >
                  {workerId}
                  <span className="msg-count">
                    {session.workers[workerId].messages.length} msgs
                  </span>
                </button>
              ))}
            </div>
            {selectedWorker && session.workers[selectedWorker] && (
              <AgentPanel 
                name={selectedWorker} 
                messages={session.workers[selectedWorker].messages}
                hideHeader
              />
            )}
          </div>
        )}
        
        {activeTab === 'mcp' && (
          <McpPanel requests={session.mcpRequests} />
        )}
      </div>
    </div>
  )
}
