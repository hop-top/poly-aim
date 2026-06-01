//! Registry: filter operations over a Provider catalog.

use crate::source::{ModelsDevSource, SourceError};
use crate::types::{Filter, Model, Provider};
use std::collections::HashMap;
use std::sync::OnceLock;

pub struct Registry {
    catalog: OnceLock<HashMap<String, Provider>>,
    source: Option<ModelsDevSource>,
}

impl Registry {
    pub fn from_catalog(catalog: HashMap<String, Provider>) -> Self {
        let lock = OnceLock::new();
        lock.set(catalog).expect("fresh OnceLock");
        Self {
            catalog: lock,
            source: None,
        }
    }

    pub fn with_source(source: ModelsDevSource) -> Self {
        Self {
            catalog: OnceLock::new(),
            source: Some(source),
        }
    }

    async fn ensure_loaded(&self) -> Result<&HashMap<String, Provider>, SourceError> {
        if let Some(c) = self.catalog.get() {
            return Ok(c);
        }
        let source = self
            .source
            .as_ref()
            .expect("no source configured and no catalog set");
        let fetched = source.fetch().await?;
        let _ = self.catalog.set(fetched);
        Ok(self.catalog.get().expect("just set"))
    }

    pub async fn models(&self, filter: Filter) -> Result<Vec<Model>, SourceError> {
        let providers = self.ensure_loaded().await?;
        let mut out: Vec<Model> = Vec::new();
        let mut keys: Vec<&String> = providers.keys().collect();
        keys.sort();
        for pid in keys {
            let p = &providers[pid];
            if !filter.provider.is_empty() && filter.provider != p.id {
                continue;
            }
            let mut model_keys: Vec<&String> = p.models.keys().collect();
            model_keys.sort();
            for mid in model_keys {
                let m = &p.models[mid];
                if matches(m, &filter) {
                    out.push(m.clone());
                }
            }
        }
        Ok(out)
    }

    pub async fn providers(&self) -> Result<Vec<Provider>, SourceError> {
        let providers = self.ensure_loaded().await?;
        let mut keys: Vec<&String> = providers.keys().collect();
        keys.sort();
        Ok(keys.into_iter().map(|k| providers[k].clone()).collect())
    }
}

impl Default for Registry {
    fn default() -> Self {
        Self::with_source(ModelsDevSource::default())
    }
}

fn matches(m: &Model, f: &Filter) -> bool {
    if !f.family.is_empty() && f.family != m.family {
        return false;
    }
    if !subset(&f.input, &m.modalities.input) {
        return false;
    }
    if !subset(&f.output, &m.modalities.output) {
        return false;
    }
    if let Some(want) = f.tool_call {
        if want != m.tool_call {
            return false;
        }
    }
    if let Some(want) = f.reasoning {
        if want != m.reasoning {
            return false;
        }
    }
    if let Some(want) = f.open_weights {
        if want != m.open_weights {
            return false;
        }
    }
    if let Some(want) = f.structured_output {
        if want != m.structured_output {
            return false;
        }
    }
    if let Some(want) = f.temperature {
        if want != m.temperature {
            return false;
        }
    }
    if !f.query.is_empty() && !query_match(m, &f.query) {
        return false;
    }
    true
}

fn subset(need: &[String], have: &[String]) -> bool {
    need.iter().all(|n| have.iter().any(|h| h == n))
}

fn query_match(m: &Model, q: &str) -> bool {
    let q = q.to_lowercase();
    m.id.to_lowercase().contains(&q) || m.name.to_lowercase().contains(&q)
}
