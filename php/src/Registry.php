<?php

declare(strict_types=1);

namespace HopTop\Aim;

final class Registry
{
    /** @var array<string, Provider>|null */
    private ?array $catalog = null;

    public function __construct(
        private ?ModelsDevSource $source = null,
    ) {
        $this->source ??= new ModelsDevSource();
    }

    /** @return list<Model> */
    public function models(Filter $filter): array
    {
        $catalog = $this->ensureLoaded();
        $out = [];
        $pids = array_keys($catalog);
        sort($pids);
        foreach ($pids as $pid) {
            if ($filter->provider !== '' && $filter->provider !== $pid) continue;
            $p = $catalog[$pid];
            $mids = array_keys($p->models);
            sort($mids);
            foreach ($mids as $mid) {
                $m = $p->models[$mid];
                if (self::matches($m, $filter)) $out[] = $m;
            }
        }
        return $out;
    }

    /** @return list<Provider> */
    public function providers(): array
    {
        $catalog = $this->ensureLoaded();
        $pids = array_keys($catalog);
        sort($pids);
        return array_map(fn($k) => $catalog[$k], $pids);
    }

    /** @return array<string, Provider> */
    private function ensureLoaded(): array
    {
        if ($this->catalog !== null) return $this->catalog;
        assert($this->source !== null);
        $this->catalog = $this->source->fetch();
        return $this->catalog;
    }

    private static function matches(Model $m, Filter $f): bool
    {
        if ($f->family !== '' && $f->family !== $m->family) return false;
        if (!self::subset($f->input,  $m->modalities->input))  return false;
        if (!self::subset($f->output, $m->modalities->output)) return false;
        if ($f->toolCall         !== null && $f->toolCall         !== $m->toolCall)         return false;
        if ($f->reasoning        !== null && $f->reasoning        !== $m->reasoning)        return false;
        if ($f->openWeights      !== null && $f->openWeights      !== $m->openWeights)      return false;
        if ($f->structuredOutput !== null && $f->structuredOutput !== $m->structuredOutput) return false;
        if ($f->temperature      !== null && $f->temperature      !== $m->temperature)      return false;
        if ($f->query !== '' && !self::queryMatch($m, $f->query)) return false;
        return true;
    }

    /**
     * @param list<string> $need
     * @param list<string> $have
     */
    private static function subset(array $need, array $have): bool
    {
        foreach ($need as $n) {
            if (!in_array($n, $have, true)) return false;
        }
        return true;
    }

    private static function queryMatch(Model $m, string $q): bool
    {
        $q = strtolower($q);
        return str_contains(strtolower($m->id), $q) || str_contains(strtolower($m->name), $q);
    }
}
