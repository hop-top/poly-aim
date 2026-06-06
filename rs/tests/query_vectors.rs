//! Cross-SDK query parser vectors. Loaded from testdata/query-vectors.json.

use hop_top_aim::{parse_query, Filter};
use serde::Deserialize;
use std::fs;
use std::path::PathBuf;

#[derive(Deserialize)]
struct Vector {
    #[serde(default)]
    description: String,
    input: String,
    #[serde(default)]
    expected: Option<ExpectedFilter>,
    #[serde(default, rename = "error")]
    should_fail: bool,
}

#[derive(Deserialize, Default)]
struct ExpectedFilter {
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

impl From<ExpectedFilter> for Filter {
    fn from(e: ExpectedFilter) -> Filter {
        Filter {
            input: e.input,
            output: e.output,
            provider: e.provider,
            family: e.family,
            tool_call: e.tool_call,
            reasoning: e.reasoning,
            open_weights: e.open_weights,
            structured_output: e.structured_output,
            temperature: e.temperature,
            query: e.query,
        }
    }
}

fn vectors_path() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .expect("repo root")
        .join("testdata/query-vectors.json")
}

#[test]
fn query_vectors_all() {
    let raw = fs::read_to_string(vectors_path()).expect("read vectors");
    let vectors: Vec<Vector> = serde_json::from_str(&raw).expect("parse vectors");
    assert!(!vectors.is_empty(), "vectors must be non-empty");

    for v in vectors {
        let got = parse_query(&v.input);
        if v.should_fail {
            assert!(
                got.is_err(),
                "{} — expected error, got {:?}",
                v.description,
                got
            );
            continue;
        }
        let want: Filter = v.expected.unwrap_or_default().into();
        let got = got.unwrap_or_else(|_| panic!("{} — parse failed", v.description));
        assert_eq!(got, want, "case: {} input={:?}", v.description, v.input);
    }
}
