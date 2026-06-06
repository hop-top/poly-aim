//! Registry filter behavior driven by shared cross-SDK vectors.

use hop_top_aim::{Filter, Provider, Registry};
use serde::Deserialize;
use std::collections::HashMap;
use std::fs;
use std::path::PathBuf;

#[derive(Deserialize, Default)]
struct FilterSpec {
    #[serde(rename = "Input", default)]
    input: Vec<String>,
    #[serde(rename = "Output", default)]
    output: Vec<String>,
    #[serde(rename = "Provider", default)]
    provider: String,
    #[serde(rename = "Family", default)]
    family: String,
    #[serde(rename = "ToolCall", default)]
    tool_call: Option<bool>,
    #[serde(rename = "Reasoning", default)]
    reasoning: Option<bool>,
    #[serde(rename = "OpenWeights", default)]
    open_weights: Option<bool>,
    #[serde(rename = "StructuredOutput", default)]
    structured_output: Option<bool>,
    #[serde(rename = "Temperature", default)]
    temperature: Option<bool>,
    #[serde(rename = "Query", default)]
    query: String,
}

impl From<FilterSpec> for Filter {
    fn from(s: FilterSpec) -> Filter {
        Filter {
            input: s.input,
            output: s.output,
            provider: s.provider,
            family: s.family,
            tool_call: s.tool_call,
            reasoning: s.reasoning,
            open_weights: s.open_weights,
            structured_output: s.structured_output,
            temperature: s.temperature,
            query: s.query,
        }
    }
}

#[derive(Deserialize)]
struct Vector {
    description: String,
    #[serde(default)]
    filter: FilterSpec,
    expected_ids: Vec<String>,
}

fn repo_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .unwrap()
        .to_path_buf()
}

fn load_catalog() -> HashMap<String, Provider> {
    let raw = fs::read_to_string(repo_root().join("testdata/api-fixture.json"))
        .expect("read api fixture");
    let mut providers: HashMap<String, Provider> =
        serde_json::from_str(&raw).expect("parse fixture");
    for (id, p) in providers.iter_mut() {
        p.id = id.clone();
        for m in p.models.values_mut() {
            m.provider = id.clone();
        }
    }
    providers
}

#[tokio::test]
async fn registry_vectors_all() {
    let catalog = load_catalog();
    let registry = Registry::from_catalog(catalog);

    let raw = fs::read_to_string(repo_root().join("testdata/registry-vectors.json"))
        .expect("read vectors");
    let vectors: Vec<Vector> = serde_json::from_str(&raw).expect("parse vectors");

    for v in vectors {
        let filter: Filter = v.filter.into();
        let models = registry.models(filter).await.expect("filter");
        let mut got: Vec<String> = models
            .iter()
            .map(|m| format!("{}/{}", m.provider, m.id))
            .collect();
        got.sort();
        let mut want = v.expected_ids.clone();
        want.sort();
        assert_eq!(got, want, "case: {}", v.description);
    }
}
