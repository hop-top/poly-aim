<?php

declare(strict_types=1);

namespace HopTop\Aim;

final class Provider
{
    /**
     * @param list<string> $env
     * @param array<string, Model> $models
     */
    public function __construct(
        public string $id = '',
        public string $name = '',
        public string $doc = '',
        public string $api = '',
        public string $npm = '',
        public array $env = [],
        public array $models = [],
    ) {}
}
