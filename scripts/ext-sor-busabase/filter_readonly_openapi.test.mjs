import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { filterReadonlyOpenAPI } from './filter_readonly_openapi.mjs'

const WRITE_METHODS = ['post', 'put', 'patch', 'delete', 'options', 'head', 'trace']

test('drops write methods and keeps GET', () => {
  const input = JSON.parse(readFileSync(new URL('./fixtures/mini-openapi.json', import.meta.url)))
  const out = filterReadonlyOpenAPI(input)
  assert.equal(out.paths['/a'].get != null, true)
  assert.equal(out.paths['/a'].post, undefined)
  assert.equal(out.paths['/b'].get != null, true)
  assert.equal(out.paths['/b'].parameters?.length, 1)
  assert.equal(out.paths['/b'].summary, 'List B')
  assert.equal(out.paths['/write-only'], undefined)

  for (const pathItem of Object.values(out.paths)) {
    for (const method of WRITE_METHODS) {
      assert.equal(pathItem[method], undefined, `unexpected ${method} on path item`)
    }
    assert.notEqual(pathItem.get, undefined)
  }
})
