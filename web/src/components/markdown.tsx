import Markdown, { type Components } from 'react-markdown'
import rehypeSanitize, { defaultSchema } from 'rehype-sanitize'
import remarkGfm from 'remark-gfm'

const schema = {
  ...defaultSchema,
  // Remote images are rendered as links: loading them would leak the reader's
  // IP and read time to whoever sent the notification.
  tagNames: (defaultSchema.tagNames ?? []).filter((t) => t !== 'img'),
}

const components: Components = {
  a: ({ href, title, children }) => (
    <a href={href} title={title} target="_blank" rel="noopener noreferrer nofollow">
      {children}
    </a>
  ),
}

/** GitHub-flavoured, sanitized markdown for notification bodies. */
export function MarkdownBody({ source }: { source: string }) {
  return (
    <div className="md">
      <Markdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[[rehypeSanitize, schema]]}
        components={components}
        urlTransform={(url) => url}
      >
        {source}
      </Markdown>
    </div>
  )
}
