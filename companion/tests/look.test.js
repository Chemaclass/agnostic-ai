import { test } from 'node:test'
import assert from 'node:assert/strict'
import { dimOf, motionOf, pausedOf } from '../frontend/look.js'

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

test('pauses only when the shell says so', () => {
  assert.equal(pausedOf({ paused: true }), 'on')
  assert.equal(pausedOf({ paused: false }), 'off')
  assert.equal(pausedOf({}), 'off')
  assert.equal(pausedOf(undefined), 'off')
})

test('a pause changes neither motion nor dim', () => {
  assert.equal(motionOf({ animate: true, dimWhenIdle: true, paused: true }), 'on')
  assert.equal(dimOf({ animate: true, dimWhenIdle: true, paused: true }), 'on')
})
