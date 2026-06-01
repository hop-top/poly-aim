//! Source HTTP fetcher tests using wiremock.

use hop_aim::ModelsDevSource;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

#[tokio::test]
async fn source_fetch_returns_providers() {
    let server = MockServer::start().await;
    let body = r#"{"openai":{"id":"openai","name":"OpenAI","models":{"gpt-4":{"id":"gpt-4","name":"GPT-4","modalities":{"input":["text"],"output":["text"]},"tool_call":true,"reasoning":false,"open_weights":false,"limit":{"context":8192}}}}}"#;
    Mock::given(method("GET"))
        .and(path("/api.json"))
        .respond_with(ResponseTemplate::new(200).set_body_string(body))
        .mount(&server)
        .await;

    let src = ModelsDevSource::builder()
        .url(format!("{}/api.json", server.uri()))
        .build();
    let providers = src.fetch().await.expect("fetch");
    assert_eq!(providers.len(), 1);
    let openai = providers.get("openai").expect("openai");
    assert_eq!(openai.name, "OpenAI");
    let model = openai.models.get("gpt-4").expect("gpt-4");
    assert_eq!(
        model.provider, "openai",
        "provider backfilled from parent key"
    );
}

#[tokio::test]
async fn source_fetch_5xx_errors() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/api.json"))
        .respond_with(ResponseTemplate::new(503))
        .mount(&server)
        .await;

    let src = ModelsDevSource::builder()
        .url(format!("{}/api.json", server.uri()))
        .build();
    let err = src.fetch().await.expect_err("should fail");
    let s = err.to_string();
    assert!(
        s.contains("503") || s.to_lowercase().contains("unavailable"),
        "got: {}",
        s
    );
}
