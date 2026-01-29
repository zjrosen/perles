import { useMemo } from 'react'
import type { AgentMessage } from '../types'
import './AgentPanel.css'

interface Props {
  name: string
  messages: AgentMessage[]
  hideHeader?: boolean
}

interface MessageGroup {
  role: string
  ts: string
  isToolGroup: boolean
  messages: AgentMessage[]
}

const roleColors: Record<string, string> = {
  'coordinator': 'var(--accent-blue)',
  'assistant': 'var(--accent-green)',
  'user': 'var(--accent-yellow)',
  'system': 'var(--text-muted)',
}

export default function AgentPanel({ name, messages, hideHeader }: Props) {
  const formatTime = (ts: string) => {
    return new Date(ts).toLocaleTimeString()
  }

  // Group consecutive tool calls from the same role
  const groupedMessages = useMemo(() => {
    const groups: MessageGroup[] = []
    
    for (const msg of messages) {
      const lastGroup = groups[groups.length - 1]
      
      // If this is a tool call and the last group is tool calls from same role, add to it
      if (
        msg.is_tool_call &&
        lastGroup?.isToolGroup &&
        lastGroup.role === msg.role
      ) {
        lastGroup.messages.push(msg)
      } else {
        // Start a new group
        groups.push({
          role: msg.role,
          ts: msg.ts,
          isToolGroup: !!msg.is_tool_call,
          messages: [msg],
        })
      }
    }
    
    return groups
  }, [messages])

  return (
    <div className="agent-panel">
      {!hideHeader && <h2>{name} Messages ({messages.length})</h2>}
      
      <div className="messages-list">
        {groupedMessages.map((group, index) => (
          <div 
            key={index} 
            className={`message-item ${group.isToolGroup ? 'tool-call' : ''}`}
          >
            <div className="message-header">
              <span 
                className="message-role"
                style={{ color: roleColors[group.role] || 'var(--text-secondary)' }}
              >
                {group.role}
              </span>
              <span className="message-time">{formatTime(group.ts)}</span>
            </div>
            
            <div className="message-content">
              {group.isToolGroup ? (
                <div className="tool-group">
                  {group.messages.map((msg, i) => (
                    <span key={i} className="tool-badge">{msg.content}</span>
                  ))}
                </div>
              ) : (
                <pre>{group.messages[0].content}</pre>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
