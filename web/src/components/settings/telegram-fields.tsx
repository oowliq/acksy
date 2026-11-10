import { useState } from 'react'
import { Field, Input } from '../form'

export function TelegramFields({ onChange }: { onChange: (url: string) => void }) {
  const [token, setToken] = useState('')
  const [chat, setChat] = useState('')
  const update = (nextToken: string, nextChat: string) => {
    setToken(nextToken)
    setChat(nextChat)
    const key = nextToken.trim()
    const id = nextChat.trim()
    const valid = /^\d+:[\w-]+$/.test(key) && /^(-?[1-9]\d*|@[\w]+)$/.test(id)
    onChange(valid ? `https://api.telegram.org/bot${key}/sendMessage?chat_id=${id}` : '')
  }
  return (
    <>
      <Field label="Bot token" hint="Use the token from @BotFather.">
        {(f) => (
          <Input
            {...f}
            required
            type="password"
            autoComplete="new-password"
            value={token}
            onChange={(e) => update(e.target.value, chat)}
          />
        )}
      </Field>
      <Field label="Chat ID" hint="Enter a numeric chat ID or @channel. For a private chat, start the bot first.">
        {(f) => (
          <Input
            {...f}
            required
            autoCapitalize="none"
            spellCheck={false}
            value={chat}
            onChange={(e) => update(token, e.target.value)}
          />
        )}
      </Field>
    </>
  )
}
