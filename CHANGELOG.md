# Changelog

## [1.11.1](https://github.com/matthisholleville/argocd-mcp/compare/v1.11.0...v1.11.1) (2026-11-20)


### Bug Fixes

* **deps:** update module github.com/mark3labs/mcp-go to v1.6.0 ([#33](https://github.com/matthisholleville/argocd-mcp/issues/33)) ([6255c7f](https://github.com/matthisholleville/argocd-mcp/commit/6255c7fc9e33196d17bf4a2548c317d193344915))

## [1.11.0](https://github.com/matthisholleville/argocd-mcp/compare/v1.10.0...v1.11.0) (2026-11-19)


### Features

* rank keyword search by operation name and term rarity, add max_results ([#31](https://github.com/matthisholleville/argocd-mcp/issues/31)) ([eeb6fd4](https://github.com/matthisholleville/argocd-mcp/commit/eeb6fd427ee671b13dffe12150441ef9c4eb0b83))

## [1.10.0](https://github.com/matthisholleville/argocd-mcp/compare/v1.9.0...v1.10.0) (2026-11-18)


### Features

* add ARGOCD_CA_BUNDLE and chart extraEnv/extraVolumes/extraVolumeMounts ([#30](https://github.com/matthisholleville/argocd-mcp/issues/30)) ([31109e5](https://github.com/matthisholleville/argocd-mcp/commit/31109e54b7c71ee697ed087efc5d5d1f4d4c2f79))


### Bug Fixes

* honor ARGOCD_TLS_INSECURE and HTTP(S)_PROXY on every outbound HTTP client ([#28](https://github.com/matthisholleville/argocd-mcp/issues/28)) ([4862573](https://github.com/matthisholleville/argocd-mcp/commit/48625730f57a583116d9f065c20ed8dc94e92bd5))


### Documentation

* remove duplicated changelog entries (1.7.0 → 1.9.0) ([#27](https://github.com/matthisholleville/argocd-mcp/issues/27)) ([5eb628f](https://github.com/matthisholleville/argocd-mcp/commit/5eb628fd590fca2e6c0651230f0d21ba31f75967))

## [1.9.0](https://github.com/matthisholleville/argocd-mcp/compare/v1.8.1...v1.9.0) (2026-11-18)


### Features

* support MCP spec 2026-07-28 (mcp-go v1.5.0) + e2e harness against real ArgoCD ([1fb0280](https://github.com/matthisholleville/argocd-mcp/commit/1fb0280d04caf62de368f4e9fcfb71f9b047c061))


### Bug Fixes

* **deps:** move to Go 1.26, update dependencies and actions, add Renovate ([e998f8c](https://github.com/matthisholleville/argocd-mcp/commit/e998f8cf9c902965348d0f8927227e805817877f))


### Documentation

* add AGENTS.md and CLAUDE.md for AI coding agents ([59c9b3c](https://github.com/matthisholleville/argocd-mcp/commit/59c9b3c588018338acb0205cb347f712fe5e46ed))
* make red/green TDD and e2e coverage mandatory in AGENTS.md ([7897ff8](https://github.com/matthisholleville/argocd-mcp/commit/7897ff84874ef3da4d8324db6f67c0cd939b8398))


### Miscellaneous

* **deps:** update go toolchain ([72ba09f](https://github.com/matthisholleville/argocd-mcp/commit/72ba09fe16e80fae0ea062b19ab0364fa332e8da))
* **deps:** update helm release ollama to v1.95.0 ([10a0588](https://github.com/matthisholleville/argocd-mcp/commit/10a0588aed7530eb20a91f155232cd8e92efa851))
* pin golang builder image by digest, log e2e bootstrap step durations ([5d1e609](https://github.com/matthisholleville/argocd-mcp/commit/5d1e6096908ce880e4ef1f4350ea353ff265c445))

## [1.8.1](https://github.com/matthisholleville/argocd-mcp/compare/v1.8.0...v1.8.1) (2026-11-18)


### Bug Fixes

* set explicit MCP annotations so read-only tools aren't flagged destructive ([9f8c56d](https://github.com/matthisholleville/argocd-mcp/commit/9f8c56dc0a36f4f4ef79e383b66cd060682bca29))


### Miscellaneous

* remove inline comments on tool annotations ([e3b53ba](https://github.com/matthisholleville/argocd-mcp/commit/e3b53ba642f11e36b495aae0e2f44b9bd399a756))

## [1.8.0](https://github.com/matthisholleville/argocd-mcp/compare/v1.7.0...v1.8.0) (2026-08-13)


### Features

* expose Prometheus metrics for tool calls ([375a70c](https://github.com/matthisholleville/argocd-mcp/commit/375a70c56ec0da6bde8c660d171fd972bc7821a7))


### Bug Fixes

* also count missing method/path as a metrics call, not just other validation failures ([db3a2c7](https://github.com/matthisholleville/argocd-mcp/commit/db3a2c7b4352de7363741fc895e683f0faa3b029))
* don't drop metrics on validation failures, don't pollute latency histogram with blocked calls ([8d3dbaf](https://github.com/matthisholleville/argocd-mcp/commit/8d3dbaf373270409cf412e84429108d18b685eb9))

## [1.7.0](https://github.com/matthisholleville/argocd-mcp/compare/v1.6.0...v1.7.0) (2026-07-29)


### Features

* add oidc auth mode for ArgoCD instances without Dex ([ae94414](https://github.com/matthisholleville/argocd-mcp/commit/ae94414fa02db747ca4114812f6f170eb8e692a9))
* terminate the provider callback in oidc mode ([01e3abc](https://github.com/matthisholleville/argocd-mcp/commit/01e3abce8387a68758f66b0173d2be94df260578))


### Bug Fixes

* resolve the OAuth provider before reporting ready ([fbb36cb](https://github.com/matthisholleville/argocd-mcp/commit/fbb36cb898b518f7b314bc97458a45782b881b80))
* validate the client redirect_uri in oidc proxy-callback mode ([8cfac07](https://github.com/matthisholleville/argocd-mcp/commit/8cfac07193868802c2fd8a16a6f9ca07a2e1f3e8))


### Documentation

* align the oidc documentation with the validation changes ([093f7b9](https://github.com/matthisholleville/argocd-mcp/commit/093f7b954400de702bf55e4b91b7b0654058630f))

## [1.6.0](https://github.com/matthisholleville/argocd-mcp/compare/v1.5.0...v1.6.0) (2026-03-30)


### Features

* add generated tools mode (TOOL_MODE=generated) ([28625c1](https://github.com/matthisholleville/argocd-mcp/commit/28625c152f935b035d2f97bf68bac475c6e1dacb))

## [1.5.0](https://github.com/matthisholleville/argocd-mcp/compare/v1.4.0...v1.5.0) (2026-03-22)


### Features

* add per-user rate limiting for execute_operation ([c05bd8b](https://github.com/matthisholleville/argocd-mcp/commit/c05bd8bc797934ce1e9236aeedfe52c4ffb2f3ea))

## [1.4.0](https://github.com/matthisholleville/argocd-mcp/compare/v1.3.0...v1.4.0) (2026-03-22)


### Features

* add production-grade Helm chart with OCI registry publishing ([145e893](https://github.com/matthisholleville/argocd-mcp/commit/145e893902a134c9cc7e5686c83ba26e9500550a))


### Bug Fixes

* add helm repo add before dependency build in CI workflows ([c7a73d5](https://github.com/matthisholleville/argocd-mcp/commit/c7a73d5cf763ad96882c43e88d37f6d75a077f52))
* handle NDJSON and non-JSON responses from ArgoCD ([67b8d1c](https://github.com/matthisholleville/argocd-mcp/commit/67b8d1c47625a9d77d93fcad0daaf4e41ff7ae51))


### Miscellaneous

* let release-please manage Chart.yaml version via extra-files ([9f74176](https://github.com/matthisholleville/argocd-mcp/commit/9f74176fbe45e3e44a5dea45a7a7d9a375ea85f3))

## [1.3.0](https://github.com/matthisholleville/argocd-mcp/compare/v1.2.0...v1.3.0) (2026-03-22)


### Features

* add /healthz and /readyz endpoints for Kubernetes probes ([f015ef1](https://github.com/matthisholleville/argocd-mcp/commit/f015ef18147e4be3b2520ec94b4da230e8e6ecf5))
* add config validation tests and fail on invalid boolean env vars ([87b5ef5](https://github.com/matthisholleville/argocd-mcp/commit/87b5ef5883ef258402d4715308648cc2a29b10ad))
* add MCP prompt templates for common ArgoCD workflows ([4f9e70a](https://github.com/matthisholleville/argocd-mcp/commit/4f9e70a9cb7c196144ca8bef912d91165ceb689b))
* make TLS certificate verification configurable via ARGOCD_TLS_INSECURE ([a2e22c1](https://github.com/matthisholleville/argocd-mcp/commit/a2e22c1f08ba12095d082c3414c8f314d1694c06))


### Bug Fixes

* add 15s timeout to Dex token HTTP client ([184485e](https://github.com/matthisholleville/argocd-mcp/commit/184485ee183a8f2f579d60319a92e5bfbad01ad1))
* add 30s startup timeout for OpenAPI spec fetch ([b5cb79d](https://github.com/matthisholleville/argocd-mcp/commit/b5cb79d7fea768f30beb5561b77f290325c5ee89))
* add debug log for malformed register request body ([064b42f](https://github.com/matthisholleville/argocd-mcp/commit/064b42fdc6432ba9877798edf521498cf98e1d9d))
* add response body size limits to prevent OOM from oversized responses ([70ae299](https://github.com/matthisholleville/argocd-mcp/commit/70ae2991b3e6b5521747f15a7afd0e0b5df5f2a9))
* add structured error logging before returning startup errors ([0bfcba6](https://github.com/matthisholleville/argocd-mcp/commit/0bfcba625bea912ab0cb15f1e90b4f405ffd2421))
* use build-injected version instead of hardcoded serverVersion ([15e2a50](https://github.com/matthisholleville/argocd-mcp/commit/15e2a50c0a0a322040bd03b0b1dc05979c974859))
* use graceful shutdown with 5s timeout for HTTP server ([29007d7](https://github.com/matthisholleville/argocd-mcp/commit/29007d7cf9f3632c86ab0e2c710a68d327c468c4))

## [1.2.0](https://github.com/matthisholleville/argocd-mcp/compare/v1.1.0...v1.2.0) (2026-03-22)


### Features

* add ALLOWED_RESOURCES to restrict exposed endpoints by resource type ([56d30e2](https://github.com/matthisholleville/argocd-mcp/commit/56d30e235c18f2e0cf06974dc48bd6bf951f725f))
* add structured audit logging for every tool call ([dd60ef9](https://github.com/matthisholleville/argocd-mcp/commit/dd60ef95a73cf95d4e29525d534969e6a09e20de))

## [1.1.0](https://github.com/matthisholleville/argocd-mcp/compare/v1.0.0...v1.1.0) (2026-03-19)


### Features

* add DISABLE_WRITE option to block disruptive operations ([0b0efca](https://github.com/matthisholleville/argocd-mcp/commit/0b0efca6686ad11844b21af1b83b5795db86bdce))

## 1.0.0 (2026-03-15)


### Features

* OpenAPI-driven ArgoCD MCP server ([6a08a2e](https://github.com/matthisholleville/argocd-mcp/commit/6a08a2ecd82944f286a8bb641e4f1a6ff70b0571))
