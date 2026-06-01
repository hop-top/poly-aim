"""ParseQuery — string query parser for aim Filter."""

from __future__ import annotations

from .types import Filter

_KNOWN_TAG_KEYS = frozenset({
    "in",
    "out",
    "provider",
    "family",
    "tool_call",
    "reasoning",
    "open_weights",
    "structured_output",
    "temperature",
})


class _Token:
    __slots__ = ("val", "quoted")

    def __init__(self, val: str, quoted: bool) -> None:
        self.val = val
        self.quoted = quoted


def _tokenise(q: str) -> list[_Token]:
    q = q.strip()
    if not q:
        return []

    tokens: list[_Token] = []
    i = 0
    n = len(q)

    while i < n:
        # skip whitespace
        if q[i] in (" ", "\t"):
            i += 1
            continue

        if q[i] == '"':
            # quoted string
            j = i + 1
            while j < n and q[j] != '"':
                j += 1
            if j >= n:
                raise ValueError("aim: unterminated quoted string in query")
            tokens.append(_Token(val=q[i + 1:j], quoted=True))
            i = j + 1
            continue

        # unquoted token — read until whitespace or quote
        j = i
        while j < n and q[j] not in (" ", "\t", '"'):
            j += 1
        raw = q[i:j]
        i = j

        if raw == ":":
            raise ValueError("aim: bare colon in query")

        tokens.append(_Token(val=raw, quoted=False))

    return tokens


def _parse_bool(s: str) -> bool:
    if s == "true":
        return True
    if s == "false":
        return False
    raise ValueError(
        f'aim: invalid bool value "{s}": must be "true" or "false"'
    )


def _apply_tag(f: Filter, key: str, val: str) -> None:
    if key == "in":
        f.input.extend(val.split(","))
    elif key == "out":
        f.output.extend(val.split(","))
    elif key == "provider":
        f.provider = val
    elif key == "family":
        f.family = val
    elif key == "tool_call":
        f.tool_call = _parse_bool(val)
    elif key == "reasoning":
        f.reasoning = _parse_bool(val)
    elif key == "open_weights":
        f.open_weights = _parse_bool(val)
    elif key == "structured_output":
        f.structured_output = _parse_bool(val)
    elif key == "temperature":
        f.temperature = _parse_bool(val)
    else:
        # Caller pre-validates against _KNOWN_TAG_KEYS so this is
        # unreachable in normal use. Surface it loudly if someone
        # adds a key to the validator but forgets to wire it here.
        raise ValueError(f'aim: unknown tag key "{key}"')


def parse_query(q: str) -> Filter:
    """Parse a string query into a Filter.

    Raises ValueError for unknown tag keys, invalid bool values, or bare colons.

    Syntax:
      key:value  — structured tag (see known keys)
      "..."      — quoted free-text; colons inside are literal
      bare token — free-text appended to Filter.query
    """
    tokens = _tokenise(q)
    f = Filter()
    free_text: list[str] = []

    for tok in tokens:
        if tok.quoted:
            free_text.append(tok.val)
            continue

        colon_idx = tok.val.find(":")
        if colon_idx == -1:
            free_text.append(tok.val)
            continue

        key = tok.val[:colon_idx]
        val = tok.val[colon_idx + 1:]

        if not key or not val:
            raise ValueError(
                f'aim: malformed tag "{tok.val}": key and value must both be non-empty'
            )

        if key not in _KNOWN_TAG_KEYS:
            raise ValueError(f'aim: unknown tag key "{key}"')

        _apply_tag(f, key, val)

    if free_text:
        f.query = " ".join(free_text)

    return f
