import type { Filter, Model, Provider } from './types.ts'
import { ModelsDevSource } from './source.ts'
import { parseQuery } from './query.ts'

// NOTE: XDG cache is skipped for the TS port — uses in-memory cache only
// (fetch once per Registry instance). See testdata/xdg.md for XDG algorithm
// used by Go and Python ports.

export interface RegistryOptions {
  url?: string
}

/** Registry provides access to the models.dev provider/model data. */
export class Registry {
  private readonly source: ModelsDevSource
  private cache: Record<string, Provider> | null = null

  constructor(opts: RegistryOptions = {}) {
    this.source = new ModelsDevSource({ url: opts.url })
  }

  /** refresh forces a re-fetch from the source. */
  async refresh(): Promise<void> {
    this.cache = await this.source.fetch()
  }

  private async load(): Promise<Record<string, Provider>> {
    if (!this.cache) {
      this.cache = await this.source.fetch()
    }
    return this.cache
  }

  /** providers returns all providers. */
  async providers(): Promise<Provider[]> {
    const data = await this.load()
    return Object.values(data)
  }

  /** models returns all models, optionally filtered. */
  async models(filter?: Filter): Promise<Model[]> {
    const data = await this.load()
    const all: Model[] = []
    for (const p of Object.values(data)) {
      for (const m of Object.values(p.models ?? {})) {
        if (m) all.push(m)
      }
    }
    if (!filter) return all
    return all.filter((m) => matchesFilter(m, filter))
  }

  /** get returns a single model by provider + model id. */
  async get(
    providerId: string,
    modelId: string,
  ): Promise<Model | undefined> {
    const data = await this.load()
    return data[providerId]?.models?.[modelId] ?? undefined
  }

  /** query parses the query string and returns matching models. */
  async query(q: string): Promise<Model[]> {
    const filter = parseQuery(q)
    return this.models(filter)
  }
}

function matchesFilter(m: Model, f: Filter): boolean {
  if (f.input?.length) {
    for (const mod of f.input) {
      if (!m.modalities.input.includes(mod)) return false
    }
  }
  if (f.output?.length) {
    for (const mod of f.output) {
      if (!m.modalities.output.includes(mod)) return false
    }
  }
  if (f.provider !== undefined && m.provider !== f.provider) return false
  if (f.family !== undefined && m.family !== f.family) return false
  if (f.toolCall !== undefined && m.tool_call !== f.toolCall) return false
  if (f.reasoning !== undefined && m.reasoning !== f.reasoning) return false
  if (f.openWeights !== undefined && m.open_weights !== f.openWeights) return false
  if (f.structuredOutput !== undefined && m.structured_output !== f.structuredOutput) return false
  if (f.temperature !== undefined && m.temperature !== f.temperature) return false
  if (f.query) {
    const q = f.query.toLowerCase()
    const idMatch = m.id.toLowerCase().includes(q)
    const nameMatch = m.name.toLowerCase().includes(q)
    if (!idMatch && !nameMatch) return false
  }
  return true
}
