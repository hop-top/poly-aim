<?php

declare(strict_types=1);

namespace HopTop\Aim\Tests;

use GuzzleHttp\Client;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Psr7\Response;
use HopTop\Aim\ModelsDevSource;
use PHPUnit\Framework\TestCase;

final class SourceTest extends TestCase
{
    public function testFetchReturnsProviders(): void
    {
        $body = '{"openai":{"id":"openai","name":"OpenAI","models":{"gpt-4":{"id":"gpt-4","name":"GPT-4","modalities":{"input":["text"],"output":["text"]},"tool_call":true,"reasoning":false,"open_weights":false,"limit":{"context":8192}}}}}';
        $mock = new MockHandler([new Response(200, [], $body)]);
        $client = new Client(['handler' => HandlerStack::create($mock)]);

        $src = new ModelsDevSource(client: $client, url: 'http://test/api.json');
        $providers = $src->fetch();
        $this->assertCount(1, $providers);
        $openai = $providers['openai'];
        $this->assertSame('OpenAI', $openai->name);
        $this->assertSame('openai', $openai->models['gpt-4']->provider, 'provider backfilled from parent key');
    }

    public function test5xxThrows(): void
    {
        $mock = new MockHandler([new Response(503)]);
        $client = new Client(['handler' => HandlerStack::create($mock)]);
        $src = new ModelsDevSource(client: $client, url: 'http://test/api.json');
        $this->expectException(\RuntimeException::class);
        $src->fetch();
    }
}
