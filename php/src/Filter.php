<?php

declare(strict_types=1);

namespace HopTop\Aim;

/**
 * Constrains a model query. All non-default fields are ANDed.
 * Tristate booleans (?bool): null = no filter, true/false = must match.
 */
final class Filter
{
    /**
     * @param list<string> $input
     * @param list<string> $output
     */
    public function __construct(
        public array $input = [],
        public array $output = [],
        public string $provider = '',
        public string $family = '',
        public ?bool $toolCall = null,
        public ?bool $reasoning = null,
        public ?bool $openWeights = null,
        public ?bool $structuredOutput = null,
        public ?bool $temperature = null,
        public string $query = '',
    ) {}
}
