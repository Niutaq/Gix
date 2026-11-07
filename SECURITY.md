# Security Policy

## Reporting a Vulnerability

We take the security of our project seriously. If you discover a security vulnerability, we appreciate your help in disclosing it to us in a responsible manner.

**Please DO NOT open a public issue on GitHub.**

Instead, please report it by sending an email to _maciekkar1305@gmail.com_.

In your report, please include:

*   A clear and concise description of the vulnerability.
*   Steps to reproduce the vulnerability.
*   The potential impact of the vulnerability.
*   Any proof-of-concept code or exploits.

## NetSecOps + FinOps controls (practical map)

Aligned with **NIS2** access / integrity themes and **ISO 27001**-style operational controls — what Gix actually enforces today:

| Risk | Control in code | Where |
|------|-----------------|--------|
| Unauth mutating API | `GIX_API_KEY` + `X-Gix-API-Key` on discover/delete | `internal/api/middleware/security.go` |
| Discover abuse / cost bomb | Rate limit 20/min/IP; post-discover harvest majors only | middleware + `handlers/discovery.go` |
| SSRF via scrape URL | `ValidateURL` + private-IP dialer | `pkg/scrapers` |
| Bad / hostile FX data | `SanitizePairForCurrency` (scrape → Redis/Timescale → UI vault) | scrapers + services + dRPC client |
| Prompt injection (LLM) | Fence neutralization + numeric/address output gates | `pkg/scrapers/llm.go`, `sanitizeLLMAddress` |
| SQL injection | Parameterized `$n` queries only | handlers / services |
| FinOps burn | Governance circuit breaker by **cantor id** (`provider_id`) | `pkg/finops/governance.go` |
| Error leakage | Generic client errors; detail in server logs | rates / finops handlers |
| Secret in URL | Gemini key via `x-goog-api-key` header | `llm.go` |

### Local vs production

* **Local:** `GIX_API_KEY` empty → mutate routes stay open (one warning log). Redis/ES without auth on loopback only.
* **Production / NIS2:** set `GIX_API_KEY`, `GIX_CORS_ORIGINS`, do not publish dRPC `:8081` on a public LoadBalancer, enable Redis AUTH / ES security if services leave localhost.

### SQL / prompt injection — mental model

* **SQL:** never concatenate user strings into queries; pass values as bind args. History/rates/discovery already do this.
* **Prompt:** page HTML is **untrusted state**, not instructions. We strip fence tokens from the page, ask for JSON/address only, then **reject** anything that fails rate bands or address shape — the model cannot “talk” past the gate into Timescale.

Island / map hover UX is intentionally separate from this control plane.

## Disclosure Policy

Once a security vulnerability is reported, we will:

1.  Confirm the vulnerability and its impact.
2.  Develop a fix for the vulnerability.
3.  Release a new version of the project with the fix.
4.  Publicly disclose the vulnerability and credit the reporter (if they wish to be credited) after the fix has been released and users have had a reasonable amount of time to update.

Thank you for helping to keep Gix secure!
