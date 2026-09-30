# @hop-top/aim — TypeScript SDK

AI model registry client backed by [models.dev](https://models.dev).
Mirrors the canonical [Go library](https://github.com/hop-top/aim).

## Install

```sh
npm install @hop-top/aim
```

## Quickstart

```ts
import { Registry } from '@hop-top/aim'

const registry = new Registry()
const models = await registry.models({ input: ['image'] })
console.log(`${models.length} models match`)
```

## License

MIT
