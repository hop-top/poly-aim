# hop-aim — Rust SDK

AI model registry client backed by [models.dev](https://models.dev).
Mirrors the canonical [Go library](https://github.com/hop-top/aim).

## Parity

API parity with Go HEAD `c6fccae` (post `Cost`/`StructuredOutput`/`Temperature`).

## Quickstart

```rust
use hop_aim::{Filter, Registry};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let registry = Registry::default();
    let filter = Filter { input: vec!["image".into()], ..Default::default() };
    let models = registry.models(filter).await?;
    println!("{} models match", models.len());
    Ok(())
}
```

## License

MIT
