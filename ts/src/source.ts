import type { Provider } from './types.ts'

export const DEFAULT_SOURCE_URL = 'https://models.dev/api.json'
export const DEFAULT_MAX_RESPONSE_SIZE = 50 * 1024 * 1024 // 50 MB

export interface ModelsDevSourceOptions {
  url?: string
  maxResponseSize?: number
}

/**
 * ModelsDevSource fetches provider data from models.dev/api.json using fetch().
 * Implements the Source interface (duck-typed).
 */
export class ModelsDevSource {
  readonly url: string
  readonly maxResponseSize: number

  constructor(opts: ModelsDevSourceOptions = {}) {
    this.url = opts.url ?? DEFAULT_SOURCE_URL
    this.maxResponseSize = opts.maxResponseSize ?? DEFAULT_MAX_RESPONSE_SIZE
  }

  async fetch(): Promise<Record<string, Provider>> {
    const resp = await globalThis.fetch(this.url, {
      headers: { Accept: 'application/json' },
    })

    if (!resp.ok) {
      throw new Error(
        `aim: fetch ${this.url}: unexpected status ${resp.status}`,
      )
    }

    const contentLength = resp.headers.get('content-length')
    if (contentLength && parseInt(contentLength, 10) > this.maxResponseSize) {
      throw new Error(
        `aim: response from ${this.url} exceeds max size (${this.maxResponseSize} bytes)`,
      )
    }

    const buf = await resp.arrayBuffer()
    if (buf.byteLength > this.maxResponseSize) {
      throw new Error(
        `aim: response from ${this.url} exceeds max size (${this.maxResponseSize} bytes)`,
      )
    }

    const raw: Record<string, Provider> = JSON.parse(
      new TextDecoder().decode(buf),
    )

    // Validate map key == Provider.id and backfill Model.provider.
    for (const [key, p] of Object.entries(raw)) {
      if (!p) {
        delete raw[key]
        continue
      }
      if (!p.id) p.id = key
      if (p.id !== key) {
        throw new Error(
          `aim: provider map key "${key}" != provider id "${p.id}"`,
        )
      }
      for (const m of Object.values(p.models ?? {})) {
        if (m) m.provider = p.id
      }
    }

    return raw
  }
}
