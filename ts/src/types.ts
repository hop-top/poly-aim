// Types mirroring the Go aim package types.
// Tristate booleans use boolean|undefined (not null): undefined = no filter.

export interface Modalities {
  input: string[]
  output: string[]
}

export interface Limits {
  context?: number
  input?: number
  output?: number
}

/** Cost holds per-token pricing in USD per 1M tokens. All fields optional. */
export interface Cost {
  input?: number
  output?: number
  cache_read?: number
  cache_write?: number
}

export interface Model {
  id: string
  name: string
  family?: string
  /** Backfilled from parent Provider.id during deserialization. */
  provider: string
  modalities: Modalities
  tool_call: boolean
  reasoning: boolean
  open_weights: boolean
  attachment?: boolean
  cost?: Cost
  structured_output: boolean
  temperature: boolean
  release_date?: string
  last_updated?: string
  knowledge?: string
  limit: Limits
}

export interface Provider {
  id: string
  name: string
  doc?: string
  api?: string
  npm?: string
  env?: string[]
  models: Record<string, Model>
}

/** Filter constrains a model query. All set fields are ANDed.
 *
 * input/output: subset match against model modalities.
 * toolCall/reasoning/openWeights/structuredOutput/temperature:
 * tristate — undefined = no filter, true/false = must match.
 */
export interface Filter {
  input?: string[]
  output?: string[]
  provider?: string
  family?: string
  /** Tristate: undefined = no filter, true/false = must match. */
  toolCall?: boolean | undefined
  /** Tristate: undefined = no filter, true/false = must match. */
  reasoning?: boolean | undefined
  /** Tristate: undefined = no filter, true/false = must match. */
  openWeights?: boolean | undefined
  /** Tristate: undefined = no filter, true/false = must match. */
  structuredOutput?: boolean | undefined
  /** Tristate: undefined = no filter, true/false = must match. */
  temperature?: boolean | undefined
  query?: string
}
