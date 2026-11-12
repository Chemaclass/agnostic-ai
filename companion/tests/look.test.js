import { test } from 'node:test'
import assert from 'node:assert/strict'
import { dimOf, motionOf, presenceOf } from '../frontend/look.js'

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

test('the presence is what the shell says, and shown otherwise', () => {
  assert.equal(presenceOf({ presence: 'hidden' }), 'hidden')
  assert.equal(presenceOf({ presence: 'faded' }), 'faded')
  assert.equal(presenceOf({ presence: 'shown' }), 'shown')
  assert.equal(presenceOf({ presence: 'elsewhere' }), 'shown')
  assert.equal(presenceOf({}), 'shown')
  assert.equal(presenceOf(undefined), 'shown')
})

test('the presence changes neither motion nor dim', () => {
  assert.equal(motionOf({ animate: true, dimWhenIdle: true, presence: 'faded' }), 'on')
  assert.equal(dimOf({ animate: true, dimWhenIdle: true, presence: 'faded' }), 'on')
})
