<?php

declare(strict_types=1);

namespace HopTop\Aim\Tests;

use HopTop\Aim\Filter;
use HopTop\Aim\Query;
use PHPUnit\Framework\TestCase;

final class QueryVectorsTest extends TestCase
{
    /** @return iterable<array{0:string, 1:string, 2:?Filter, 3:bool}> */
    public static function vectors(): iterable
    {
        $raw = file_get_contents(__DIR__ . '/../../testdata/query-vectors.json');
        self::assertIsString($raw);
        $data = json_decode($raw, true, flags: JSON_THROW_ON_ERROR);
        foreach ($data as $i => $v) {
            $name = $v['description'] ?? "vector#$i";
            $shouldFail = $v['error'] ?? false;
            $expected = $shouldFail ? null : self::buildExpected($v['expected'] ?? []);
            yield $name => [$name, $v['input'], $expected, $shouldFail];
        }
    }

    /** @param array<string, mixed> $e */
    private static function buildExpected(array $e): Filter
    {
        return new Filter(
            input: $e['Input'] ?? [],
            output: $e['Output'] ?? [],
            provider: $e['Provider'] ?? '',
            family: $e['Family'] ?? '',
            toolCall: $e['ToolCall'] ?? null,
            reasoning: $e['Reasoning'] ?? null,
            openWeights: $e['OpenWeights'] ?? null,
            structuredOutput: $e['StructuredOutput'] ?? null,
            temperature: $e['Temperature'] ?? null,
            query: $e['Query'] ?? '',
        );
    }

    #[\PHPUnit\Framework\Attributes\DataProvider('vectors')]
    public function testVector(string $desc, string $input, ?Filter $expected, bool $shouldFail): void
    {
        if ($shouldFail) {
            try {
                Query::parse($input);
                $this->fail("$desc: expected error, none thrown");
            } catch (\InvalidArgumentException) {
                $this->expectNotToPerformAssertions();
            }
            return;
        }
        $got = Query::parse($input);
        $this->assertEquals($expected, $got, $desc);
    }
}
