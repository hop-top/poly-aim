<?php

declare(strict_types=1);

namespace HopTop\Aim;

final class Modalities
{
    /**
     * @param list<string> $input
     * @param list<string> $output
     */
    public function __construct(
        public array $input = [],
        public array $output = [],
    ) {}
}
