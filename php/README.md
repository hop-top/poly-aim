# hop-top/aim — PHP SDK

AI model registry client backed by [models.dev](https://models.dev).
Mirrors the canonical [Go library](https://github.com/hop-top/aim).

## Parity

API parity with Go HEAD `c6fccae` (post `Cost`/`StructuredOutput`/`Temperature`).

## Requires

PHP 8.2+. Composer.

## Quickstart

```php
use HopTop\Aim\Registry;
use HopTop\Aim\Filter;

$registry = new Registry();
$filter = new Filter(input: ['image']);
$models = $registry->models($filter);
echo count($models) . " models match\n";
```

## License

MIT
