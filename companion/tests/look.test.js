import { test } from 'node:test'
import assert from 'node:assert/strict'
import { dimOf, hiddenOf, motionOf } from '../frontend/look.js'

test('stops motion only when the person turned animation off', () => {
  assert.equal(motionOf({ animate: false }), 'off')
  assert.equal(motionOf({ animate: true }), 'on')
})

test('keeps motion on when the appearance could not be read', () => {
  assert.equal(motionOf(undefined), 'on')
  assert.equal(motionOf({}), 'on')
})

test('dims only when the person turned it on', () => {
  assert.equal(dimOf({ dimWhenIdle: true }), 'on')
  assert.equal(dimOf({ dimWhenIdle: false }), 'off')
  assert.equal(dimOf({}), 'off')
  assert.equal(dimOf(undefined), 'off')
})

test('dim and motion do not change each other', () => {
  assert.equal(motionOf({ animate: false, dimWhenIdle: true }), 'off')
  assert.equal(dimOf({ animate: false, dimWhenIdle: true }), 'on')
  assert.equal(motionOf({ animate: true, dimWhenIdle: true }), 'on')
})

test('hidden only when the shell says so', () => {
  assert.equal(hiddenOf({ hidden: true }), 'on')
  assert.equal(hiddenOf({ hidden: false }), 'off')
  assert.equal(hiddenOf({}), 'off')
  assert.equal(hiddenOf(undefined), 'off')
})

test('hiding changes neither motion nor dim', () => {
  assert.equal(motionOf({ animate: true, dimWhenIdle: true, hidden: true }), 'on')
  assert.equal(dimOf({ animate: true, dimWhenIdle: true, hidden: true }), 'on')
})
