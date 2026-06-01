// Cross-SDK parser vectors driven by testdata/query-vectors.json.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import { parseQuery } from './query.ts'
import type { Filter } from './types.ts'

const here = dirname(fileURLToPath(import.meta.url))
// ts/src/ → ts/ → <repo>/ → <repo>/testdata/query-vectors.json
const fixture = join(here, '..', '..', 'testdata', 'query-vectors.json')

interface Vector {
  description: string
  input: string
  expected?: Record<string, unknown>
  error?: boolean
}

/** Map Go-style expected field names to TS Filter keys (sparse — only set
 *  fields present in the fixture row, mirroring parseQuery's return shape). */
function expectedToFilter(expected: Record<string, unknown>): Filter {
  const f: Filter = {}
  if ('Input' in expected) f.input = expected['Input'] as string[]
  if ('Output' in expected) f.output = expected['Output'] as string[]
  if ('Provider' in expected) f.provider = expected['Provider'] as string
  if ('Family' in expected) f.family = expected['Family'] as string
  if ('ToolCall' in expected) f.toolCall = expected['ToolCall'] as boolean
  if ('Reasoning' in expected) f.reasoning = expected['Reasoning'] as boolean
  if ('OpenWeights' in expected) f.openWeights = expected['OpenWeights'] as boolean
  if ('StructuredOutput' in expected) f.structuredOutput = expected['StructuredOutput'] as boolean
  if ('Temperature' in expected) f.temperature = expected['Temperature'] as boolean
  if ('Query' in expected) f.query = expected['Query'] as string
  return f
}

const vectors: Vector[] = JSON.parse(readFileSync(fixture, 'utf8'))

for (const v of vectors) {
  test(v.description, () => {
    if (v.error) {
      assert.throws(() => parseQuery(v.input), (err) => err instanceof Error)
      return
    }
    const got = parseQuery(v.input)
    const want = expectedToFilter(v.expected ?? {})
    assert.deepStrictEqual(got, want)
  })
}
