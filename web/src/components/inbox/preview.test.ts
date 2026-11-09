import { describe, expect, it } from 'vitest'
import { previewText } from './preview'

describe('previewText', () => {
  it('strips markdown but keeps identifiers intact', () => {
    expect(previewText('`binary_sensor.front_door` has been open')).toBe('binary_sensor.front_door has been open')
    expect(previewText('**bold** and _em_ and ~~gone~~')).toBe('bold and em and gone')
    expect(previewText('# Title\n\n- item\n```\ncode\n```\n[link](https://x)')).toBe('Title item link')
    expect(previewText('| a | b |')).toBe('a b')
  })
})
