<?php

declare(strict_types=1);

namespace HopTop\Aim;

/** Query DSL parser. Mirrors Go `query.go`. */
final class Query
{
    private const KNOWN_TAG_KEYS = [
        'in', 'out', 'provider', 'family',
        'tool_call', 'reasoning', 'open_weights',
        'structured_output', 'temperature',
    ];

    public static function parse(string $q): Filter
    {
        $f = new Filter();
        $tokens = self::tokenise(trim($q));
        $free = [];

        foreach ($tokens as $tok) {
            if ($tok['quoted']) {
                $free[] = $tok['val'];
                continue;
            }
            $val = $tok['val'];
            $colonPos = strpos($val, ':');
            if ($colonPos === false) {
                $free[] = $val;
                continue;
            }
            $key = substr($val, 0, $colonPos);
            $rest = substr($val, $colonPos + 1);
            if ($key === '' || $rest === '') {
                throw new \InvalidArgumentException("aim: empty key or value around colon");
            }
            if (!in_array($key, self::KNOWN_TAG_KEYS, true)) {
                throw new \InvalidArgumentException("aim: unknown tag key \"$key\"");
            }
            self::applyTag($f, $key, $rest);
        }
        if ($free !== []) {
            $f->query = implode(' ', $free);
        }
        return $f;
    }

    /** @return list<array{val:string, quoted:bool}> */
    private static function tokenise(string $q): array
    {
        if ($q === '') return [];
        $tokens = [];
        $n = strlen($q);
        $i = 0;
        while ($i < $n) {
            $ch = $q[$i];
            if ($ch === ' ' || $ch === "\t") { $i++; continue; }
            if ($ch === '"') {
                $j = $i + 1;
                while ($j < $n && $q[$j] !== '"') $j++;
                if ($j >= $n) throw new \InvalidArgumentException("aim: unterminated quoted string in query");
                $tokens[] = ['val' => substr($q, $i + 1, $j - $i - 1), 'quoted' => true];
                $i = $j + 1;
                continue;
            }
            $start = $i;
            while ($i < $n && $q[$i] !== ' ' && $q[$i] !== "\t" && $q[$i] !== '"') $i++;
            $raw = substr($q, $start, $i - $start);
            if ($raw === ':') throw new \InvalidArgumentException("aim: bare colon in query");
            $tokens[] = ['val' => $raw, 'quoted' => false];
        }
        return $tokens;
    }

    private static function applyTag(Filter $f, string $key, string $val): void
    {
        match ($key) {
            'in'  => $f->input  = [...$f->input,  ...explode(',', $val)],
            'out' => $f->output = [...$f->output, ...explode(',', $val)],
            'provider' => $f->provider = $val,
            'family'   => $f->family   = $val,
            'tool_call'         => $f->toolCall         = self::parseBool($val),
            'reasoning'         => $f->reasoning         = self::parseBool($val),
            'open_weights'      => $f->openWeights       = self::parseBool($val),
            'structured_output' => $f->structuredOutput  = self::parseBool($val),
            'temperature'       => $f->temperature       = self::parseBool($val),
            default => throw new \InvalidArgumentException("aim: unknown tag key \"$key\""),
        };
    }

    private static function parseBool(string $s): bool
    {
        return match ($s) {
            'true' => true,
            'false' => false,
            default => throw new \InvalidArgumentException("aim: invalid bool value \"$s\": must be \"true\" or \"false\""),
        };
    }
}
