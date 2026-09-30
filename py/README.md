# hop-top-aim — Python SDK

AI model registry client backed by [models.dev](https://models.dev).
Mirrors the canonical [Go library](https://github.com/hop-top/aim).

## Install

```sh
pip install hop-top-aim
```

## Quickstart

```python
import asyncio

from hop.aim import Filter, Registry


async def main() -> None:
    registry = Registry()
    models = await registry.models(Filter(input=["image"]))
    print(f"{len(models)} models match")


asyncio.run(main())
```

## License

MIT
