const pad = (n: number) => String(n).padStart(2, '0')

const sameDay = (a: Date, b: Date) =>
  a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()

/** Compact list time: 10:42 today, "Mon 14:02" this week, "3 Nov" this year, "3 Nov 2025" older. */
export function listTime(iso: string, now = new Date()): string {
  const d = new Date(iso)
  if (sameDay(d, now)) return `${pad(d.getHours())}:${pad(d.getMinutes())}`
  const days = (now.getTime() - d.getTime()) / 86_400_000
  if (days < 6 && days > 0) {
    return `${d.toLocaleDateString('en-GB', { weekday: 'short' })} ${pad(d.getHours())}:${pad(d.getMinutes())}`
  }
  const opts: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short' }
  if (d.getFullYear() !== now.getFullYear()) opts.year = 'numeric'
  return d.toLocaleDateString('en-GB', opts)
}

/** Full timestamp for the reading pane. */
export function fullTime(iso: string): string {
  return new Date(iso).toLocaleString('en-GB', {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export interface SnoozePreset {
  id: string
  label: string
  until: (now: Date) => Date
}

const at = (now: Date, addDays: number, hour: number) => {
  const d = new Date(now)
  d.setDate(d.getDate() + addDays)
  d.setHours(hour, 0, 0, 0)
  return d
}

export const snoozePresets: SnoozePreset[] = [
  {
    id: '1h',
    label: 'In 1 hour',
    until: (n) => new Date(n.getTime() + 3_600_000),
  },
  {
    id: '4h',
    label: 'In 4 hours',
    until: (n) => new Date(n.getTime() + 4 * 3_600_000),
  },
  {
    id: 'evening',
    label: 'This evening',
    until: (n) => (n.getHours() < 18 ? at(n, 0, 18) : at(n, 1, 18)),
  },
  { id: 'tomorrow', label: 'Tomorrow morning', until: (n) => at(n, 1, 9) },
  {
    id: 'monday',
    label: 'Next Monday',
    until: (n) => at(n, (8 - n.getDay()) % 7 || 7, 9),
  },
]

/** Short label for when a snooze ends, e.g. "until 18:00" or "until Tue 09:00". */
export function snoozeLabel(iso: string, now = new Date()): string {
  return `until ${listTime(iso, now)}`
}
