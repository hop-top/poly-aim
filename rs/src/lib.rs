//! AI model registry client backed by models.dev.
//!
//! Mirrors the Go library `hop.top/aim`.

mod query;
mod registry;
mod source;
mod types;

pub use query::{parse_query, ParseError};
pub use registry::Registry;
pub use source::{
    ModelsDevSource, ModelsDevSourceBuilder, SourceError, DEFAULT_MAX_RESPONSE_SIZE,
    DEFAULT_SOURCE_URL,
};
pub use types::{Cost, Filter, Limits, Modalities, Model, Provider};
