# Security Model

Aegis has three primary security boundaries:

- the public transfer API
- the internal operational API for wallets and reconciliation
- the outbound webhook client that calls user-provided callback URLs

This document describes the current controls around those boundaries and the assumptions they rely on.

## Internal Operational Authentication

Wallet and reconciliation routes are protected by:

- `INTERNAL_AUTH_HEADER`
- `INTERNAL_AUTH_API_KEY`

Behavior:

- requests must present the configured header name with the expected shared secret
- if the internal API key is unset, protected routes fail closed with `503 Service Unavailable`

This keeps internal workflows unavailable by default until an operator configures an explicit secret.

## Webhook Signing

When `WEBHOOK_SIGNING_SECRET` is set, outbound webhooks include:

- `X-Aegis-Timestamp`
- `X-Aegis-Signature`

The signature format is `v1=<hex hmac sha256>` and is computed over:

`<timestamp>.<raw_body>`

Receivers can use this to authenticate that the payload came from Aegis and to apply their own replay-window checks against the timestamp.

## Callback URL Validation Policy

At transfer creation time, callback URLs:

- must use `http` or `https`
- must not include user credentials
- reject `localhost`, `.localhost`, `.local`, `.internal`, and private or local IP literals by default
- can be restricted further with `CALLBACK_URL_ALLOWED_HOSTS`
- can allow private targets only when `CALLBACK_URL_ALLOW_PRIVATE_TARGETS=true`

This filters obvious unsafe callback targets before a transfer is accepted.

## Dispatch-Time Callback Hardening

The worker does not trust create-time validation alone. At webhook dispatch time:

- the hostname is checked against the allowlist policy again
- DNS results are rejected if any resolved IP is loopback, private, link-local, multicast, or unspecified
- redirect targets are re-validated before the client follows them

These controls reduce hostname-to-private-IP bypasses and similar SSRF tricks that can appear after the original request was accepted.

## Response Body Storage Limits

Webhook response bodies are normalized before persistence:

- bodies are truncated and sanitized before storage
- `WEBHOOK_RESPONSE_BODY_MAX_BYTES` defaults to `512`
- null bytes are removed
- invalid UTF-8 is normalized before the response body is saved

This keeps operational debugging data bounded and reduces the chance that response persistence becomes a storage or parsing footgun.

## SSRF Boundary and Infrastructure Assumptions

The callback controls above are application-layer SSRF mitigations. They materially reduce obvious abuse paths, but they are not a substitute for infrastructure controls.

Production deployments should still enforce outbound egress restrictions such as:

- network policies or security groups that limit reachable destinations
- DNS and proxy controls aligned with the callback allowlist policy
- environment-specific decisions about whether private callback targets should ever be enabled

`CALLBACK_URL_ALLOW_PRIVATE_TARGETS=true` should only be used in trusted internal environments where outbound paths are already constrained.

## Recommended Security Configuration

- Set a non-default `INTERNAL_AUTH_API_KEY` before exposing internal routes.
- Set `WEBHOOK_SIGNING_SECRET` so receivers can verify callbacks.
- Keep `CALLBACK_URL_ALLOW_PRIVATE_TARGETS=false` unless the deployment is intentionally internal-only.
- Use `CALLBACK_URL_ALLOWED_HOSTS` when callbacks should be restricted to known partner domains.

For how these controls interact with retry, ownership, and at-least-once delivery, continue with the [Reliability Model](reliability-model.md).
