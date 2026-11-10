import { describe, expect, it } from 'vitest'
import type { Notification } from './api'
import { listQueryString } from './api'
import { listTime, snoozePresets } from './format'
import { displayOrder, filterKey, groupEntries, listParams, markAllReadRequest, parseInboxSearch, topicSwatch } from './inbox'

const n = (id: string, group_key?: string) => ({ id, group_key }) as Notification

describe('groupEntries', () => {
  it('keeps singles as rows and trays groups at their newest member', () => {
    const entries = groupEntries([n('5', 'ups'), n('4'), n('3', 'ups'), n('2', 'solo'), n('1', 'ups')])
    expect(entries.map((e) => (e.kind === 'item' ? e.item.id : `tray:${e.groupKey}`))).toEqual(['tray:ups', '4', '2'])
    const tray = entries[0]
    if (tray?.kind !== 'tray') throw new Error('expected a tray')
    expect(tray.items.map((i) => i.id)).toEqual(['5', '3', '1'])
    expect(displayOrder(entries).map((i) => i.id)).toEqual(['5', '3', '1', '4', '2'])
  })
})

describe('parseInboxSearch', () => {
  it('accepts valid params and drops junk', () => {
    expect(
      parseInboxSearch({
        view: 'archived',
        topic: 't1',
        all: 'true',
        prio: '4',
        n: 'x',
      }),
    ).toEqual({
      view: 'archived',
      topic: 't1',
      all: true,
      prio: 4,
      n: 'x',
    })
    expect(parseInboxSearch({ view: 'trash', prio: 9, topic: '' })).toEqual({})
    expect(parseInboxSearch({ view: 'inbox' })).toEqual({})
  })
})

describe('topicSwatch', () => {
  it('uses a saved hex color when present', () => {
    expect(topicSwatch('abc', '#123abc')).toBe('#123abc')
  })
  it('is stable and never uses reserved inks', () => {
    expect(topicSwatch('abc')).toBe(topicSwatch('abc'))
    for (const id of ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j']) {
      expect(topicSwatch(id)).not.toMatch(/red|peach|lavender|green|maroon|flamingo|rosewater|pink/)
    }
  })
})

describe('listTime', () => {
  const now = new Date(2026, 10, 9, 12, 0)
  it('formats by distance', () => {
    expect(listTime(new Date(2026, 10, 9, 10, 42).toISOString(), now)).toBe('10:42')
    expect(listTime(new Date(2026, 10, 7, 14, 2).toISOString(), now)).toBe('Sat 14:02')
    expect(listTime(new Date(2026, 9, 3, 9, 0).toISOString(), now)).toBe('3 Oct')
    expect(listTime(new Date(2025, 9, 3, 9, 0).toISOString(), now)).toBe('3 Oct 2025')
  })
})

describe('snoozePresets', () => {
  const mon = new Date(2026, 10, 9, 20, 0) // Monday evening
  const get = (id: string) => snoozePresets.find((p) => p.id === id)!.until(mon)
  it('computes future times', () => {
    expect(get('evening').getDate()).toBe(10)
    expect(get('tomorrow').getHours()).toBe(9)
    const monday = get('monday')
    expect(monday.getDay()).toBe(1)
    expect(monday.getDate()).toBe(16)
    for (const p of snoozePresets) expect(p.until(mon).getTime()).toBeGreaterThan(mon.getTime())
  })
})

describe('search and group state', () => {
  it('reads q (trimmed) and group from the URL', () => {
    expect(parseInboxSearch({ q: '  disk ', group: 'backup' })).toEqual({
      q: 'disk',
      group: 'backup',
    })
    expect(parseInboxSearch({ q: '   ' })).toEqual({})
  })

  it('passes q and group to the API', () => {
    expect(listParams({ q: 'disk', group: 'g' })).toMatchObject({
      q: 'disk',
      groupKey: 'g',
    })
    expect(listQueryString({ q: 'бэкап' })).toBe('?q=%D0%B1%D1%8D%D0%BA%D0%B0%D0%BF')
  })

  it('changes the filter key with the search, so selection resets', () => {
    expect(filterKey({ q: 'a' })).not.toBe(filterKey({ q: 'b' }))
    expect(filterKey({ group: 'a' })).not.toBe(filterKey({}))
  })

  it('marks all read with the same filters the list shows', () => {
    expect(markAllReadRequest({})).toEqual({
      action: 'read',
      view: 'inbox',
      all: true,
    })
    expect(markAllReadRequest({ q: 'disk', prio: 4, group: 'g', view: 'snoozed' }, 'alice/ci')).toEqual({
      action: 'read',
      view: 'snoozed',
      topic: 'alice/ci',
      q: 'disk',
      min_priority: 4,
      group_key: 'g',
    })
  })

  it('lists a group flat when it is the whole view', () => {
    const entries = groupEntries([n('2', 'g'), n('1', 'g')], { flat: true })
    expect(entries.map((e) => e.kind)).toEqual(['item', 'item'])
  })
})
