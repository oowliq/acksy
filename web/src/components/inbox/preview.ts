/** One-line plain-text preview of a markdown body. */
export function previewText(md: string, max = 160): string {
  return (
    md
      .replace(/```[\s\S]*?```/g, ' ')
      .replace(/`([^`]*)`/g, '$1')
      .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')
      .replace(/^\s{0,3}(#{1,6}|>|[-*+]|\d+\.)\s+/gm, '')
      .replace(/\*{1,3}|~~|\|/g, '')
      // Underscore emphasis only at word edges, so snake_case survives.
      .replace(/(^|[\s(])_{1,3}(?=\S)/g, '$1')
      .replace(/(\S)_{1,3}(?=[\s).,;:!?]|$)/g, '$1')
      .replace(/\s+/g, ' ')
      .trim()
      .slice(0, max)
  )
}
