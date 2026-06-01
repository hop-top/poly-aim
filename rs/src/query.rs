//! Query string parser. Mirrors Go `query.go`.

use crate::types::Filter;

#[derive(Debug, thiserror::Error)]
#[error("aim: {0}")]
pub struct ParseError(pub String);

const KNOWN_TAG_KEYS: &[&str] = &[
    "in",
    "out",
    "provider",
    "family",
    "tool_call",
    "reasoning",
    "open_weights",
    "structured_output",
    "temperature",
];

struct Token {
    val: String,
    quoted: bool,
}

fn tokenise(q: &str) -> Result<Vec<Token>, ParseError> {
    let q = q.trim();
    if q.is_empty() {
        return Ok(vec![]);
    }
    let bytes = q.as_bytes();
    let n = bytes.len();
    let mut tokens = Vec::new();
    let mut i = 0;
    while i < n {
        match bytes[i] {
            b' ' | b'\t' => {
                i += 1;
            }
            b'"' => {
                let start = i + 1;
                let mut j = start;
                while j < n && bytes[j] != b'"' {
                    j += 1;
                }
                if j >= n {
                    return Err(ParseError("unterminated quoted string in query".into()));
                }
                tokens.push(Token {
                    val: String::from_utf8_lossy(&bytes[start..j]).into_owned(),
                    quoted: true,
                });
                i = j + 1;
            }
            _ => {
                let start = i;
                while i < n && !matches!(bytes[i], b' ' | b'\t' | b'"') {
                    i += 1;
                }
                let raw = String::from_utf8_lossy(&bytes[start..i]).into_owned();
                if raw == ":" {
                    return Err(ParseError("bare colon in query".into()));
                }
                tokens.push(Token {
                    val: raw,
                    quoted: false,
                });
            }
        }
    }
    Ok(tokens)
}

fn parse_bool(s: &str) -> Result<bool, ParseError> {
    match s {
        "true" => Ok(true),
        "false" => Ok(false),
        _ => Err(ParseError(format!(
            "invalid bool value \"{}\": must be \"true\" or \"false\"",
            s
        ))),
    }
}

fn apply_tag(f: &mut Filter, key: &str, val: &str) -> Result<(), ParseError> {
    match key {
        "in" => f.input.extend(val.split(',').map(String::from)),
        "out" => f.output.extend(val.split(',').map(String::from)),
        "provider" => f.provider = val.to_string(),
        "family" => f.family = val.to_string(),
        "tool_call" => f.tool_call = Some(parse_bool(val)?),
        "reasoning" => f.reasoning = Some(parse_bool(val)?),
        "open_weights" => f.open_weights = Some(parse_bool(val)?),
        "structured_output" => f.structured_output = Some(parse_bool(val)?),
        "temperature" => f.temperature = Some(parse_bool(val)?),
        _ => return Err(ParseError(format!("unknown tag key \"{}\"", key))),
    }
    Ok(())
}

pub fn parse_query(q: &str) -> Result<Filter, ParseError> {
    let mut f = Filter::default();
    let tokens = tokenise(q)?;
    let mut free = Vec::new();

    for tok in tokens {
        if tok.quoted {
            free.push(tok.val);
            continue;
        }
        if let Some((key, val)) = tok.val.split_once(':') {
            if key.is_empty() || val.is_empty() {
                return Err(ParseError("empty key or value around colon".into()));
            }
            if !KNOWN_TAG_KEYS.contains(&key) {
                return Err(ParseError(format!("unknown tag key \"{}\"", key)));
            }
            apply_tag(&mut f, key, val)?;
        } else {
            free.push(tok.val);
        }
    }

    if !free.is_empty() {
        f.query = free.join(" ");
    }
    Ok(f)
}
