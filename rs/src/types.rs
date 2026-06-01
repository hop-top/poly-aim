//! Core types mirroring the models.dev/api.json wire format.

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Modalities describes the input and output modalities a model supports.
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct Modalities {
    #[serde(default)]
    pub input: Vec<String>,
    #[serde(default)]
    pub output: Vec<String>,
}

/// Limits holds token/context window sizes for a model.
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct Limits {
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub context: i64,
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub input: i64,
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub output: i64,
}

fn is_zero_i64(n: &i64) -> bool {
    *n == 0
}

/// Cost holds per-token pricing in USD per 1M tokens.
/// All fields optional; many open-weight models omit cost entirely.
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq)]
pub struct Cost {
    #[serde(default, skip_serializing_if = "is_zero_f64")]
    pub input: f64,
    #[serde(default, skip_serializing_if = "is_zero_f64")]
    pub output: f64,
    #[serde(default, skip_serializing_if = "is_zero_f64", rename = "cache_read")]
    pub cache_read: f64,
    #[serde(default, skip_serializing_if = "is_zero_f64", rename = "cache_write")]
    pub cache_write: f64,
}

fn is_zero_f64(n: &f64) -> bool {
    *n == 0.0
}

/// Model is a single LLM entry within a provider.
///
/// `provider` is populated from the parent Provider.ID during
/// deserialization, NOT from the wire format itself.
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq)]
pub struct Model {
    pub id: String,
    pub name: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub family: String,
    #[serde(skip)]
    pub provider: String,
    #[serde(default)]
    pub modalities: Modalities,
    #[serde(default, rename = "tool_call")]
    pub tool_call: bool,
    #[serde(default)]
    pub reasoning: bool,
    #[serde(default, rename = "open_weights")]
    pub open_weights: bool,
    #[serde(default, skip_serializing_if = "is_false")]
    pub attachment: bool,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cost: Option<Cost>,
    #[serde(
        default,
        skip_serializing_if = "is_false",
        rename = "structured_output"
    )]
    pub structured_output: bool,
    #[serde(default, skip_serializing_if = "is_false")]
    pub temperature: bool,
    #[serde(
        default,
        skip_serializing_if = "String::is_empty",
        rename = "release_date"
    )]
    pub release_date: String,
    #[serde(
        default,
        skip_serializing_if = "String::is_empty",
        rename = "last_updated"
    )]
    pub last_updated: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub knowledge: String,
    #[serde(default)]
    pub limit: Limits,
}

fn is_false(b: &bool) -> bool {
    !*b
}

/// Provider is the top-level entry in the models.dev registry.
/// The map key in the wire format MUST equal Provider.id.
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq)]
pub struct Provider {
    pub id: String,
    pub name: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub doc: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub api: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub npm: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub env: Vec<String>,
    #[serde(default)]
    pub models: HashMap<String, Model>,
}

/// Filter constrains a model query. All non-default fields are ANDed.
///
/// Tristate fields use `Option<bool>`: `None` = no filter,
/// `Some(true)`/`Some(false)` = must match.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct Filter {
    pub input: Vec<String>,
    pub output: Vec<String>,
    pub provider: String,
    pub family: String,
    pub tool_call: Option<bool>,
    pub reasoning: Option<bool>,
    pub open_weights: Option<bool>,
    pub structured_output: Option<bool>,
    pub temperature: Option<bool>,
    pub query: String,
}
