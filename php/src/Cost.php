<?php

declare(strict_types=1);

namespace HopTop\Aim;

final class Cost
{
    public function __construct(
        public float $input = 0.0,
        public float $output = 0.0,
        public float $cacheRead = 0.0,
        public float $cacheWrite = 0.0,
    ) {}
}
