package aim

import (
	"context"
	"net"
	"net/url"
	"sort"
	"strings"
)

// Provider facts are derived from the catalog fields of a [Provider]; none
// needs a network call. The derivation rules are pinned by the shared
// conformance fixtures testdata/provider-fixture.json and
// testdata/provider-facts-vectors.json, which every SDK port runs.

// providerAliases is the curated alias overlay: alias -> models.dev
// provider id. Aliases cover names other tools use for a catalog provider.
// An alias never equals a catalog provider id; when the catalog grows a
// provider of the same name, [Registry.Provider] returns the real one.
var providerAliases = map[string]string{
	"fireworks": "fireworks-ai",
	"gemini":    "google",
	"together":  "togetherai",
}

// npmProtocols maps a models.dev "npm" package to the protocol it speaks,
// named after the AI SDK provider. Packages not listed have no known
// protocol: [Provider.Protocol] returns "" rather than guessing.
var npmProtocols = map[string]string{
	"@ai-sdk/amazon-bedrock":          "amazon-bedrock",
	"@ai-sdk/anthropic":               "anthropic",
	"@ai-sdk/azure":                   "azure",
	"@ai-sdk/cerebras":                "cerebras",
	"@ai-sdk/cohere":                  "cohere",
	"@ai-sdk/deepinfra":               "deepinfra",
	"@ai-sdk/gateway":                 "gateway",
	"@ai-sdk/google":                  "google",
	"@ai-sdk/google-vertex":           "google-vertex",
	"@ai-sdk/google-vertex/anthropic": "google-vertex-anthropic",
	"@ai-sdk/groq":                    "groq",
	"@ai-sdk/mistral":                 "mistral",
	"@ai-sdk/openai":                  "openai",
	"@ai-sdk/openai-compatible":       "openai-compatible",
	"@ai-sdk/perplexity":              "perplexity",
	"@ai-sdk/togetherai":              "togetherai",
	"@ai-sdk/vercel":                  "vercel",
	"@ai-sdk/xai":                     "xai",
	"@openrouter/ai-sdk-provider":     "openrouter",
}

// CanonicalProviderID returns the models.dev provider id for name, which
// may be an id or a curated alias ("gemini" -> "google"). Any other name,
// known or not, is returned unchanged. Matching is exact (case-sensitive),
// like [Filter.Provider]. It reads no catalog, so it works offline.
func CanonicalProviderID(name string) string {
	if id, ok := providerAliases[name]; ok {
		return id
	}
	return name
}

// ProviderAliases returns the curated aliases of a provider, given its id
// or one of its aliases, sorted. It returns nil when there are none. The
// slice is a copy. It reads no catalog, so it works offline.
func ProviderAliases(name string) []string {
	id := CanonicalProviderID(name)
	var out []string
	for alias, target := range providerAliases {
		if target == id {
			out = append(out, alias)
		}
	}
	sort.Strings(out)
	return out
}

// Aliases returns the curated aliases of p (see [ProviderAliases]).
func (p Provider) Aliases() []string {
	return ProviderAliases(p.ID)
}

// KeyVars returns the env vars in p.Env that hold a credential: names
// ending in "_KEY" (so also "_API_KEY"), "_APIKEY", "_PAT" or "_TOKEN".
// Catalog order is kept, so the first name is the one to prefer. It
// returns nil when there are none.
func (p Provider) KeyVars() []string {
	return filterEnv(p.Env, true)
}

// Settings returns the env vars in p.Env that are not key vars (see
// [Provider.KeyVars]): region, project, resource name, endpoint and the
// like. Catalog order is kept. It returns nil when there are none.
func (p Provider) Settings() []string {
	return filterEnv(p.Env, false)
}

// IsLocal reports whether p's default base URL ([Provider.API]) points at
// a loopback host: "localhost" or a name under ".localhost" (RFC 6761), or
// a loopback IP (127.0.0.0/8, ::1). A local provider's key is optional.
// An empty or unparsable API is not local.
func (p Provider) IsLocal() bool {
	u, err := url.Parse(p.API)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Protocol returns the protocol p speaks, derived from its npm package:
// "openai-compatible", "openai", "anthropic", "google", "mistral" and so
// on. It returns "" when the package is absent or not known to aim.
func (p Provider) Protocol() string {
	return npmProtocols[p.NPM]
}

// Provider returns the provider whose id or curated alias is name. A
// catalog id takes precedence over an alias of the same name. Returns
// (provider, true, nil) when found and (zero, false, nil) when not.
func (r *Registry) Provider(ctx context.Context, name string) (Provider, bool, error) {
	if err := r.ensureLoaded(ctx); err != nil {
		return Provider{}, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.providers[name]; ok && p != nil {
		return *p, true, nil
	}
	if id, ok := providerAliases[name]; ok {
		if p, ok := r.providers[id]; ok && p != nil {
			return *p, true, nil
		}
	}
	return Provider{}, false, nil
}

// keyVarSuffixes mark an env var name as a credential. "_KEY" also
// covers "_API_KEY".
var keyVarSuffixes = []string{"_KEY", "_APIKEY", "_PAT", "_TOKEN"}

// isKeyVar reports whether an env var name holds a credential.
func isKeyVar(name string) bool {
	for _, suffix := range keyVarSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// filterEnv returns the names in env whose key-var status equals keys.
func filterEnv(env []string, keys bool) []string {
	var out []string
	for _, name := range env {
		if isKeyVar(name) == keys {
			out = append(out, name)
		}
	}
	return out
}
