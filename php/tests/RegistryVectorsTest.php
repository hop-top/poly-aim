<?php

declare(strict_types=1);

namespace HopTop\Aim\Tests;

use HopTop\Aim\Filter;
use HopTop\Aim\ModelsDevSource;
use HopTop\Aim\Registry;
use PHPUnit\Framework\TestCase;

final class RegistryVectorsTest extends TestCase
{
    /** @return iterable<array{0:string, 1:Filter, 2:list<string>}> */
    public static function vectors(): iterable
    {
        $raw = file_get_contents(__DIR__ . '/../../testdata/registry-vectors.json');
        self::assertIsString($raw);
        $data = json_decode($raw, true, flags: JSON_THROW_ON_ERROR);
        foreach ($data as $v) {
            $f = $v['filter'] ?? [];
            $filter = new Filter(
                input:            $f['Input']            ?? [],
                output:           $f['Output']           ?? [],
                provider:         $f['Provider']         ?? '',
                family:           $f['Family']           ?? '',
                toolCall:         $f['ToolCall']         ?? null,
                reasoning:        $f['Reasoning']        ?? null,
                openWeights:      $f['OpenWeights']      ?? null,
                structuredOutput: $f['StructuredOutput'] ?? null,
                temperature:      $f['Temperature']      ?? null,
                query:            $f['Query']            ?? '',
            );
            yield $v['description'] => [$v['description'], $filter, $v['expected_ids']];
        }
    }

    /**
     * @param list<string> $expectedIds
     */
    #[\PHPUnit\Framework\Attributes\DataProvider('vectors')]
    public function testVector(string $desc, Filter $filter, array $expectedIds): void
    {
        $source = self::sourceFromFixture();
        $registry = new Registry(source: $source);
        $models = $registry->models($filter);
        $got = array_map(fn($m) => "{$m->provider}/{$m->id}", $models);
        sort($got);
        sort($expectedIds);
        $this->assertSame($expectedIds, $got, $desc);
    }

    private static function sourceFromFixture(): ModelsDevSource
    {
        $body = file_get_contents(__DIR__ . '/../../testdata/api-fixture.json');
        $mock = new \GuzzleHttp\Handler\MockHandler([new \GuzzleHttp\Psr7\Response(200, [], $body)]);
        $client = new \GuzzleHttp\Client(['handler' => \GuzzleHttp\HandlerStack::create($mock)]);
        return new ModelsDevSource(client: $client, url: 'http://fixture/api.json');
    }
}
