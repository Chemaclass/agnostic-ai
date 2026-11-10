import { test } from 'node:test'
import assert from 'node:assert/strict'
import { motionOf } from '../frontend/look.js'

test('stops motion only when the person turned animation off', () => {
  assert.equal(motionOf({ animate: false }), 'off')
  assert.equal(motionOf({ animate: true }), 'on')
})

test('keeps motion on when the appearance could not be read', () => {
  assert.equal(motionOf(undefined), 'on')
  assert.equal(motionOf({}), 'on')
})
