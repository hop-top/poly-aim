//! HTTP-backed models.dev source.

use crate::types::Provider;
use std::collections::HashMap;
use std::time::Duration;

pub const DEFAULT_SOURCE_URL: &str = "https://models.dev/api.json";
pub const DEFAULT_MAX_RESPONSE_SIZE: u64 = 50 * 1024 * 1024;
const DEFAULT_TIMEOUT_SECS: u64 = 30;

#[derive(Debug, thiserror::Error)]
pub enum SourceError {
    #[error("aim: http error: {0}")]
    Http(#[from] reqwest::Error),
    #[error("aim: upstream returned status {status}")]
    Status { status: u16 },
    #[error("aim: response exceeds max size {max} bytes")]
    TooLarge { max: u64 },
    #[error("aim: invalid json: {0}")]
    Json(#[from] serde_json::Error),
}

pub struct ModelsDevSource {
    client: reqwest::Client,
    url: String,
    max_size: u64,
}

impl ModelsDevSource {
    pub fn builder() -> ModelsDevSourceBuilder {
        ModelsDevSourceBuilder::default()
    }

    pub async fn fetch(&self) -> Result<HashMap<String, Provider>, SourceError> {
        let resp = self.client.get(&self.url).send().await?;
        let status = resp.status();
        if !status.is_success() {
            return Err(SourceError::Status {
                status: status.as_u16(),
            });
        }
        let bytes = resp.bytes().await?;
        if bytes.len() as u64 > self.max_size {
            return Err(SourceError::TooLarge { max: self.max_size });
        }
        let mut providers: HashMap<String, Provider> = serde_json::from_slice(&bytes)?;
        // Backfill model.provider from parent key.
        for (id, provider) in providers.iter_mut() {
            for model in provider.models.values_mut() {
                model.provider = id.clone();
            }
            provider.id = id.clone();
        }
        Ok(providers)
    }
}

impl Default for ModelsDevSource {
    fn default() -> Self {
        Self::builder().build()
    }
}

#[derive(Default)]
pub struct ModelsDevSourceBuilder {
    url: Option<String>,
    max_size: Option<u64>,
    timeout: Option<Duration>,
}

impl ModelsDevSourceBuilder {
    pub fn url(mut self, u: impl Into<String>) -> Self {
        self.url = Some(u.into());
        self
    }
    pub fn max_size(mut self, n: u64) -> Self {
        self.max_size = Some(n);
        self
    }
    pub fn timeout(mut self, d: Duration) -> Self {
        self.timeout = Some(d);
        self
    }
    pub fn build(self) -> ModelsDevSource {
        let timeout = self
            .timeout
            .unwrap_or(Duration::from_secs(DEFAULT_TIMEOUT_SECS));
        let client = reqwest::Client::builder()
            .timeout(timeout)
            .build()
            .expect("reqwest client");
        ModelsDevSource {
            client,
            url: self.url.unwrap_or_else(|| DEFAULT_SOURCE_URL.to_string()),
            max_size: self.max_size.unwrap_or(DEFAULT_MAX_RESPONSE_SIZE),
        }
    }
}
