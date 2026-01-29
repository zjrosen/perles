import { useEffect } from 'react'
import './MarkdownModal.css'

interface Props {
  title: string
  content: string
  onClose: () => void
}

export default function MarkdownModal({ title, content, onClose }: Props) {
  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose()
      }
    }
    window.addEventListener('keydown', handleEsc)
    return () => window.removeEventListener('keydown', handleEsc)
  }, [onClose])

  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      onClose()
    }
  }

  // Simple markdown rendering - handles headers, bold, code blocks, lists, tables
  const renderMarkdown = (md: string) => {
    const lines = md.split('\n')
    const elements: React.ReactNode[] = []
    let i = 0
    let key = 0

    while (i < lines.length) {
      const line = lines[i]

      // Code block
      if (line.startsWith('```')) {
        const codeLines: string[] = []
        i++
        while (i < lines.length && !lines[i].startsWith('```')) {
          codeLines.push(lines[i])
          i++
        }
        elements.push(<pre key={key++} className="md-code-block">{codeLines.join('\n')}</pre>)
        i++
        continue
      }

      // Table detection
      if (line.includes('|') && i + 1 < lines.length && lines[i + 1].includes('---')) {
        const tableLines: string[] = [line]
        i++
        while (i < lines.length && lines[i].includes('|')) {
          tableLines.push(lines[i])
          i++
        }
        elements.push(renderTable(tableLines, key++))
        continue
      }

      // Headers
      if (line.startsWith('# ')) {
        elements.push(<h1 key={key++}>{formatInline(line.slice(2))}</h1>)
      } else if (line.startsWith('## ')) {
        elements.push(<h2 key={key++}>{formatInline(line.slice(3))}</h2>)
      } else if (line.startsWith('### ')) {
        elements.push(<h3 key={key++}>{formatInline(line.slice(4))}</h3>)
      } else if (line.startsWith('#### ')) {
        elements.push(<h4 key={key++}>{formatInline(line.slice(5))}</h4>)
      }
      // Horizontal rule
      else if (line.match(/^-{3,}$/)) {
        elements.push(<hr key={key++} />)
      }
      // List items
      else if (line.match(/^[-*] /)) {
        elements.push(<li key={key++}>{formatInline(line.slice(2))}</li>)
      } else if (line.match(/^\d+\. /)) {
        elements.push(<li key={key++}>{formatInline(line.replace(/^\d+\. /, ''))}</li>)
      }
      // Empty line
      else if (line.trim() === '') {
        elements.push(<div key={key++} className="md-spacer" />)
      }
      // Regular paragraph
      else {
        elements.push(<p key={key++}>{formatInline(line)}</p>)
      }

      i++
    }

    return elements
  }

  const formatInline = (text: string): React.ReactNode => {
    // Handle inline code, bold, and links
    const parts: React.ReactNode[] = []
    let remaining = text
    let partKey = 0

    while (remaining.length > 0) {
      // Inline code
      const codeMatch = remaining.match(/`([^`]+)`/)
      // Bold
      const boldMatch = remaining.match(/\*\*([^*]+)\*\*/)

      const matches = [
        codeMatch ? { type: 'code', match: codeMatch, index: codeMatch.index! } : null,
        boldMatch ? { type: 'bold', match: boldMatch, index: boldMatch.index! } : null,
      ].filter(Boolean).sort((a, b) => a!.index - b!.index)

      if (matches.length === 0) {
        parts.push(remaining)
        break
      }

      const first = matches[0]!
      if (first.index > 0) {
        parts.push(remaining.slice(0, first.index))
      }

      if (first.type === 'code') {
        parts.push(<code key={partKey++} className="md-inline-code">{first.match[1]}</code>)
      } else if (first.type === 'bold') {
        parts.push(<strong key={partKey++}>{first.match[1]}</strong>)
      }

      remaining = remaining.slice(first.index + first.match[0].length)
    }

    return parts.length === 1 ? parts[0] : parts
  }

  const renderTable = (lines: string[], tableKey: number) => {
    const parseRow = (line: string) =>
      line.split('|').map(c => c.trim()).filter(c => c.length > 0)

    const headers = parseRow(lines[0])
    const rows = lines.slice(2).map(parseRow) // Skip header separator line

    return (
      <table key={tableKey} className="md-table">
        <thead>
          <tr>
            {headers.map((h, i) => <th key={i}>{formatInline(h)}</th>)}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, ri) => (
            <tr key={ri}>
              {row.map((cell, ci) => <td key={ci}>{formatInline(cell)}</td>)}
            </tr>
          ))}
        </tbody>
      </table>
    )
  }

  return (
    <div className="modal-backdrop" onClick={handleBackdropClick}>
      <div className="modal-content markdown-modal">
        <div className="modal-header">
          <h2>{title}</h2>
          <button className="modal-close" onClick={onClose}>×</button>
        </div>
        <div className="modal-body markdown-body">
          {renderMarkdown(content)}
        </div>
      </div>
    </div>
  )
}
