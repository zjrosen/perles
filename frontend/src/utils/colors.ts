// Generate a consistent color from a string
export function hashColor(str: string): string {
  // Special cases
  if (str.toLowerCase() === 'coordinator') {
    return 'var(--accent-purple)'
  }
  
  // Extract number from worker ID if present (e.g., "worker-12" -> 12)
  const match = str.match(/(\d+)/)
  let index: number
  
  if (match) {
    // Use the number directly for sequential workers
    index = parseInt(match[1], 10)
  } else {
    // Fall back to hash for non-numeric IDs
    let hash = 5381
    for (let i = 0; i < str.length; i++) {
      hash = ((hash << 5) + hash) ^ str.charCodeAt(i)
    }
    index = Math.abs(hash)
  }
  
  // Use golden angle (137.508°) to maximize hue separation
  // This guarantees no two adjacent indices have similar hues
  const hue = (index * 137.508) % 360
  const saturation = 70
  const lightness = 65
  
  return `hsl(${hue.toFixed(0)}, ${saturation}%, ${lightness}%)`
}

// Get initials from an agent ID (e.g., "worker-1" -> "W1", "coordinator" -> "C")
export function getInitials(id: string): string {
  if (id.toLowerCase() === 'coordinator') return 'C'
  
  const match = id.match(/^(\w)\w*-?(\d*)$/)
  if (match) {
    return (match[1].toUpperCase() + (match[2] || '')).slice(0, 2)
  }
  return id.slice(0, 2).toUpperCase()
}
