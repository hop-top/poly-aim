<?php

declare(strict_types=1);

namespace HopTop\Aim;

final class Model
{
    public function __construct(
        public string $id = '',
        public string $name = '',
        public string $family = '',
        /** Backfilled from parent Provider->id; not in wire format. */
        public string $provider = '',
        public Modalities $modalities = new Modalities(),
        public bool $toolCall = false,
        public bool $reasoning = false,
        public bool $openWeights = false,
        public bool $attachment = false,
        public ?Cost $cost = null,
        public bool $structuredOutput = false,
        public bool $temperature = false,
        public string $releaseDate = '',
        public string $lastUpdated = '',
        public string $knowledge = '',
        public Limits $limit = new Limits(),
    ) {}
}
