<?php

declare(strict_types=1);

namespace HopTop\Aim;

use GuzzleHttp\Client;
use GuzzleHttp\ClientInterface;

final class ModelsDevSource
{
    public const DEFAULT_SOURCE_URL = 'https://models.dev/api.json';
    public const DEFAULT_MAX_RESPONSE_SIZE = 50 * 1024 * 1024;
    private const DEFAULT_TIMEOUT_SECS = 30;

    public function __construct(
        private ?ClientInterface $client = null,
        private string $url = self::DEFAULT_SOURCE_URL,
        private int $maxSize = self::DEFAULT_MAX_RESPONSE_SIZE,
    ) {
        $this->client ??= new Client(['timeout' => self::DEFAULT_TIMEOUT_SECS]);
    }

    /** @return array<string, Provider> */
    public function fetch(): array
    {
        assert($this->client !== null);
        $resp = $this->client->request('GET', $this->url);
        $status = $resp->getStatusCode();
        if ($status < 200 || $status >= 300) {
            throw new \RuntimeException("aim: upstream returned status $status");
        }
        $bytes = (string) $resp->getBody();
        if (strlen($bytes) > $this->maxSize) {
            throw new \RuntimeException("aim: response exceeds max size {$this->maxSize}");
        }
        $raw = json_decode($bytes, true, flags: JSON_THROW_ON_ERROR);

        $out = [];
        foreach ($raw as $pid => $pdata) {
            $models = [];
            foreach ($pdata['models'] ?? [] as $mid => $mdata) {
                $models[$mid] = self::buildModel($mid, (string) $pid, $mdata);
            }
            $out[(string) $pid] = new Provider(
                id: (string) $pid,
                name: (string) ($pdata['name'] ?? ''),
                doc: (string) ($pdata['doc'] ?? ''),
                api: (string) ($pdata['api'] ?? ''),
                npm: (string) ($pdata['npm'] ?? ''),
                env: $pdata['env'] ?? [],
                models: $models,
            );
        }
        return $out;
    }

    /** @param array<string, mixed> $m */
    private static function buildModel(string $id, string $providerId, array $m): Model
    {
        $mods = $m['modalities'] ?? [];
        $modalities = new Modalities(
            input:  $mods['input']  ?? [],
            output: $mods['output'] ?? [],
        );
        $limit = new Limits(
            context: (int) (($m['limit'] ?? [])['context'] ?? 0),
            input:   (int) (($m['limit'] ?? [])['input']   ?? 0),
            output:  (int) (($m['limit'] ?? [])['output']  ?? 0),
        );
        $cost = null;
        if (isset($m['cost'])) {
            $c = $m['cost'];
            $cost = new Cost(
                input:      (float) ($c['input']       ?? 0),
                output:     (float) ($c['output']      ?? 0),
                cacheRead:  (float) ($c['cache_read']  ?? 0),
                cacheWrite: (float) ($c['cache_write'] ?? 0),
            );
        }
        return new Model(
            id: $id,
            name: (string) ($m['name'] ?? ''),
            family: (string) ($m['family'] ?? ''),
            provider: $providerId,
            modalities: $modalities,
            toolCall:         (bool) ($m['tool_call']         ?? false),
            reasoning:        (bool) ($m['reasoning']         ?? false),
            openWeights:      (bool) ($m['open_weights']      ?? false),
            attachment:       (bool) ($m['attachment']        ?? false),
            cost: $cost,
            structuredOutput: (bool) ($m['structured_output'] ?? false),
            temperature:      (bool) ($m['temperature']       ?? false),
            releaseDate: (string) ($m['release_date'] ?? ''),
            lastUpdated: (string) ($m['last_updated'] ?? ''),
            knowledge:   (string) ($m['knowledge']    ?? ''),
            limit: $limit,
        );
    }
}
