<?php

declare(strict_types=1);

namespace HopTop\Aim;

final class Limits
{
    public function __construct(
        public int $context = 0,
        public int $input = 0,
        public int $output = 0,
    ) {}
}
