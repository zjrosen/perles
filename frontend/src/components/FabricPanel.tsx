import { useState, useMemo } from 'react'
import type { FabricEvent } from '../types'
import { hashColor } from '../utils/colors'
import './FabricPanel.css'

interface Props {
  events: FabricEvent[]
}

interface Channel {
  id: string
  slug: string
  title: string
  purpose: string
  createdAt: string
  messageCount: number
}

interface Message {
  id: string
  channelId: string
  parentId?: string
  createdBy: string
  content: string
  kind?: string
  timestamp: string
  type: 'message' | 'reply'
  mentions: string[]
  seq: number
}

interface Thread {
  parentMessage: Message
  replies: Message[]
}

function getInitialSidebarTab(): 'events' | 'messages' {
  const params = new URLSearchParams(window.location.search)
  const subtab = params.get('subtab')
  if (subtab === 'events' || subtab === 'messages') {
    return subtab
  }
  return 'messages'
}

function getInitialChannelId(): string | null {
  const params = new URLSearchParams(window.location.search)
  return params.get('channel')
}

export default function FabricPanel({ events }: Props) {
  const [selectedChannelId, setSelectedChannelIdState] = useState<string | null>(getInitialChannelId)
  const [selectedThread, setSelectedThread] = useState<Thread | null>(null)
  const [sidebarTab, setSidebarTabState] = useState<'events' | 'messages'>(getInitialSidebarTab)
  const [expandedEvents, setExpandedEvents] = useState<Set<number>>(new Set())

  const setSidebarTab = (tab: 'events' | 'messages') => {
    setSidebarTabState(tab)
    const url = new URL(window.location.href)
    url.searchParams.set('subtab', tab)
    window.history.replaceState({}, '', url.toString())
  }

  const setSelectedChannelId = (channelId: string | null) => {
    setSelectedChannelIdState(channelId)
    const url = new URL(window.location.href)
    if (channelId) {
      url.searchParams.set('channel', channelId)
    } else {
      url.searchParams.delete('channel')
    }
    window.history.replaceState({}, '', url.toString())
  }

  // Extract channels and messages from events
  const { channels, messages } = useMemo(() => {
    const channelMap = new Map<string, Channel>()
    const messageList: Message[] = []

    for (const event of events) {
      const e = event.event
      
      if (e.type === 'channel.created' && e.thread) {
        channelMap.set(e.channel_id!, {
          id: e.channel_id!,
          slug: e.thread.slug || 'unknown',
          title: e.thread.title || 'Untitled',
          purpose: e.thread.purpose || '',
          createdAt: e.thread.created_at || event.timestamp,
          messageCount: 0,
        })
      }
      
      if ((e.type === 'message.posted' || e.type === 'reply.posted') && e.thread) {
        const msg: Message = {
          id: e.thread.id,
          channelId: e.channel_id!,
          parentId: e.parent_id,
          createdBy: e.thread.created_by || 'unknown',
          content: e.thread.content || '',
          kind: e.thread.kind,
          timestamp: e.thread.created_at || event.timestamp,
          type: e.type === 'reply.posted' ? 'reply' : 'message',
          mentions: e.mentions || [],
          seq: e.thread.seq,
        }
        messageList.push(msg)
        
        const channel = channelMap.get(e.channel_id!)
        if (channel) {
          channel.messageCount++
        }
      }
    }

    const sortedChannels = Array.from(channelMap.values()).sort((a, b) => {
      const order = ['root', 'system', 'tasks', 'planning', 'general']
      const aIdx = order.indexOf(a.slug)
      const bIdx = order.indexOf(b.slug)
      if (aIdx !== -1 && bIdx !== -1) return aIdx - bIdx
      if (aIdx !== -1) return -1
      if (bIdx !== -1) return 1
      return a.slug.localeCompare(b.slug)
    })

    return { channels: sortedChannels, messages: messageList }
  }, [events])

  // Group messages into threads using parent_id
  const threads = useMemo(() => {
    if (!selectedChannelId) return []
    
    // Build a map of ALL messages for parent lookup
    const allMessageMap = new Map<string, Message>()
    for (const msg of messages) {
      allMessageMap.set(msg.id, msg)
    }
    
    // Helper to find root channel for a message (walk up parent chain)
    const getRootChannelId = (msg: Message): string | undefined => {
      if (msg.channelId) return msg.channelId
      if (msg.parentId) {
        const parent = allMessageMap.get(msg.parentId)
        if (parent) return getRootChannelId(parent)
      }
      return undefined
    }
    
    // Get all messages belonging to this channel (including nested replies)
    const channelMessages = messages
      .filter(m => getRootChannelId(m) === selectedChannelId)
      .sort((a, b) => a.seq - b.seq)
    
    // Group replies by their direct parent_id
    const repliesByParent = new Map<string, Message[]>()
    const parentMessages: Message[] = []
    
    for (const msg of channelMessages) {
      if (msg.type === 'reply' && msg.parentId) {
        // This is a reply with a parent
        const replies = repliesByParent.get(msg.parentId) || []
        replies.push(msg)
        repliesByParent.set(msg.parentId, replies)
      } else {
        // This is a parent message (or a reply without parent_id, treat as parent)
        parentMessages.push(msg)
      }
    }
    
    // Recursively collect all descendant replies for a message
    const collectAllReplies = (messageId: string): Message[] => {
      const directReplies = repliesByParent.get(messageId) || []
      const allReplies: Message[] = []
      for (const reply of directReplies) {
        allReplies.push(reply)
        allReplies.push(...collectAllReplies(reply.id))
      }
      return allReplies
    }
    
    // Build threads from parent messages with all nested replies flattened
    const threadList: Thread[] = parentMessages.map(parent => ({
      parentMessage: parent,
      replies: collectAllReplies(parent.id).sort((a, b) => a.seq - b.seq)
    }))
    
    return threadList
  }, [messages, selectedChannelId])

  const selectedChannel = channels.find(c => c.id === selectedChannelId)

  // Auto-select first channel with messages (only if no valid channel selected)
  if (channels.length > 0 && !selectedChannel) {
    const firstWithMessages = channels.find(c => c.messageCount > 0) || channels[0]
    setSelectedChannelId(firstWithMessages.id)
  }

  const formatTime = (ts: string) => {
    const date = new Date(ts)
    return date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
  }

  const formatDate = (ts: string) => {
    const date = new Date(ts)
    return date.toLocaleDateString([], { month: 'short', day: 'numeric' })
  }

  const getAgentColor = (agent: string): string => {
    return hashColor(agent)
  }

  const getChannelIcon = (slug: string): string => {
    const icons: Record<string, string> = {
      'root': '🏠',
      'system': '⚙️',
      'tasks': '📋',
      'planning': '📐',
      'general': '💬',
    }
    return icons[slug] || '#'
  }

  const getEventColor = (type: string): string => {
    const colors: Record<string, string> = {
      'channel.created': 'var(--accent-purple)',
      'message.posted': 'var(--accent-blue)',
      'reply.posted': 'var(--accent-green)',
      'acked': 'var(--accent-yellow)',
      'subscribed': 'var(--accent-orange)',
    }
    return colors[type] || 'var(--text-muted)'
  }

  const getReplyAvatars = (replies: Message[]) => {
    const authors = [...new Set(replies.map(r => r.createdBy))]
    return authors.slice(0, 3)
  }

  return (
    <div className="fabric-panel-slack">
      {/* Channel Sidebar */}
      <aside className="channel-sidebar">
        <div className="sidebar-header">
          <h3>Channels</h3>
          <span className="channel-count">{channels.length}</span>
        </div>
        <nav className="channel-list">
          {channels.map(channel => (
            <button
              key={channel.id}
              className={`channel-item ${selectedChannelId === channel.id ? 'active' : ''}`}
              onClick={() => {
                setSelectedChannelId(channel.id)
                setSelectedThread(null)
              }}
            >
              <span className="channel-icon">{getChannelIcon(channel.slug)}</span>
              <span className="channel-name">{channel.title}</span>
              {channel.messageCount > 0 && (
                <span className="channel-badge">{channel.messageCount}</span>
              )}
            </button>
          ))}
        </nav>
        
        <div className="sidebar-stats">
          <div className="stat">
            <span className="stat-value">{events.length}</span>
            <span className="stat-label">events</span>
          </div>
          <div className="stat">
            <span className="stat-value">{messages.length}</span>
            <span className="stat-label">messages</span>
          </div>
        </div>
      </aside>

      {/* Message Area */}
      <main className={`message-area ${selectedThread ? 'with-thread' : ''}`}>
        {/* Content Area Tabs */}
        <div className="content-tabs">
          <button
            className={`content-tab ${sidebarTab === 'messages' ? 'active' : ''}`}
            onClick={() => setSidebarTab('messages')}
          >
            Messages
          </button>
          <button
            className={`content-tab ${sidebarTab === 'events' ? 'active' : ''}`}
            onClick={() => setSidebarTab('events')}
          >
            Events
          </button>
        </div>

        {sidebarTab === 'messages' ? (
          selectedChannel ? (
            <>
              <header className="channel-header">
                <div className="channel-title">
                  <span className="channel-icon-large">{getChannelIcon(selectedChannel.slug)}</span>
                  <div>
                    <h2>{selectedChannel.title}</h2>
                    <p className="channel-purpose">{selectedChannel.purpose}</p>
                  </div>
                </div>
              </header>

              <div className="message-list">
                {threads.length === 0 ? (
                  <div className="empty-channel">
                    <p>No messages in this channel</p>
                  </div>
                ) : (
                  threads.map((thread) => (
                    <div 
                      key={thread.parentMessage.id} 
                      className={`message-item ${selectedThread?.parentMessage.id === thread.parentMessage.id ? 'selected' : ''}`}
                    >
                      <div className="message-avatar" style={{ background: getAgentColor(thread.parentMessage.createdBy) }}>
                        {thread.parentMessage.createdBy.charAt(0).toUpperCase()}
                      </div>
                      <div className="message-body">
                        <div className="message-header">
                          <span className="message-author" style={{ color: getAgentColor(thread.parentMessage.createdBy) }}>
                            {thread.parentMessage.createdBy}
                          </span>
                          <span className="message-time">{formatTime(thread.parentMessage.timestamp)}</span>
                        </div>
                        <div className="message-content">
                          {thread.parentMessage.content}
                        </div>
                        
                        {/* Reply indicator */}
                        {thread.replies.length > 0 && (
                          <button 
                            className="reply-indicator"
                            onClick={() => setSelectedThread(thread)}
                          >
                            <div className="reply-avatars">
                              {getReplyAvatars(thread.replies).map((author, i) => (
                                <div 
                                  key={i}
                                  className="reply-avatar"
                                  style={{ background: getAgentColor(author) }}
                                >
                                  {author.charAt(0).toUpperCase()}
                                </div>
                              ))}
                            </div>
                            <span className="reply-count">
                              {thread.replies.length} {thread.replies.length === 1 ? 'reply' : 'replies'}
                            </span>
                            <span className="reply-preview">
                              Last reply {formatDate(thread.replies[thread.replies.length - 1].timestamp)} at {formatTime(thread.replies[thread.replies.length - 1].timestamp)}
                            </span>
                          </button>
                        )}
                      </div>
                    </div>
                  ))
                )}
              </div>
            </>
          ) : (
            <div className="no-channel-selected">
              <p>Select a channel to view messages</p>
            </div>
          )
        ) : (
          <div className="events-list">
            {events.map((event, idx) => {
              const author = event.event.thread?.created_by || event.event.agent_id
              const threadId = event.event.thread?.id
              // Don't show mentions for acked events - those are just message IDs
              const mentions = event.event.type === 'acked' 
                ? [] 
                : (event.event.mentions || event.event.thread?.mentions || [])
              return (
                <div key={idx} className="event-item">
                  <button 
                    className="event-header"
                    onClick={() => {
                      const newExpanded = new Set(expandedEvents)
                      if (newExpanded.has(idx)) {
                        newExpanded.delete(idx)
                      } else {
                        newExpanded.add(idx)
                      }
                      setExpandedEvents(newExpanded)
                    }}
                  >
                    <span className="event-expand">{expandedEvents.has(idx) ? '▼' : '▶'}</span>
                    <span className="event-type" style={{ color: getEventColor(event.event.type) }}>
                      {event.event.type}
                    </span>
                    {threadId && (
                      <span className="event-thread-id">{threadId}</span>
                    )}
                    {author && (
                      <span className="event-author" style={{ color: getAgentColor(author) }}>
                        {author}
                      </span>
                    )}
                    {mentions.length > 0 && (
                      <span className="event-mentions">
                        {mentions.map((m, i) => (
                          <span key={i} className="event-mention">@{m}</span>
                        ))}
                      </span>
                    )}
                    <span className="event-time">{formatTime(event.timestamp)}</span>
                  </button>
                  {expandedEvents.has(idx) && (
                    <pre className="event-detail">
                      {JSON.stringify(event.event, null, 2)}
                    </pre>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </main>

      {/* Thread Panel */}
      {selectedThread && (
        <aside className="thread-panel">
          <header className="thread-header">
            <h3>Thread</h3>
            <button className="close-thread" onClick={() => setSelectedThread(null)}>
              ✕
            </button>
          </header>
          
          <div className="thread-content">
            {/* Parent message */}
            <div className="thread-message parent">
              <div className="message-avatar" style={{ background: getAgentColor(selectedThread.parentMessage.createdBy) }}>
                {selectedThread.parentMessage.createdBy.charAt(0).toUpperCase()}
              </div>
              <div className="message-body">
                <div className="message-header">
                  <span className="message-author" style={{ color: getAgentColor(selectedThread.parentMessage.createdBy) }}>
                    {selectedThread.parentMessage.createdBy}
                  </span>
                  <span className="message-time">{formatTime(selectedThread.parentMessage.timestamp)}</span>
                </div>
                <div className="message-content">
                  {selectedThread.parentMessage.content}
                </div>
              </div>
            </div>

            {/* Reply count divider */}
            <div className="thread-divider">
              <span>{selectedThread.replies.length} {selectedThread.replies.length === 1 ? 'reply' : 'replies'}</span>
            </div>

            {/* Replies */}
            {selectedThread.replies.map((reply) => (
              <div key={reply.id} className="thread-message reply">
                <div className="message-avatar" style={{ background: getAgentColor(reply.createdBy) }}>
                  {reply.createdBy.charAt(0).toUpperCase()}
                </div>
                <div className="message-body">
                  <div className="message-header">
                    <span className="message-author" style={{ color: getAgentColor(reply.createdBy) }}>
                      {reply.createdBy}
                    </span>
                    <span className="message-time">{formatTime(reply.timestamp)}</span>
                  </div>
                  <div className="message-content">
                    {reply.content}
                  </div>
                  {reply.mentions.length > 0 && (
                    <div className="message-mentions">
                      {reply.mentions.map((m, i) => (
                        <span key={i} className="mention">@{m}</span>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        </aside>
      )}
    </div>
  )
}
