import { describe, expect, it } from 'vitest'
import { apiProxy } from '../vite.config'

describe('the /api proxy', () => {
  it('keeps the client host, so the server sees a same-origin request', () => {
    expect(apiProxy['/api']).toEqual({ target: 'http://127.0.0.1:4310', changeOrigin: false })
  })
})
