# Changelog

## [0.3.0](https://github.com/aaronflorey/docker-dns-sync/compare/v0.2.1...v0.3.0) (2026-07-19)


### Features

* **01-01:** implement config-driven startup and cancellable runtime ([e17220b](https://github.com/aaronflorey/docker-dns-sync/commit/e17220b17f0ea1a30db811adca5fd88b477547b3))
* **01-02:** add config validation and secret resolution ([dbd1a3b](https://github.com/aaronflorey/docker-dns-sync/commit/dbd1a3b1b27ecf683b340fb08b59c21adce69f87))
* **01-03:** add provider contracts and runtime factories ([a48260f](https://github.com/aaronflorey/docker-dns-sync/commit/a48260ffd494b0f05f3657d64991a990ebef4b16))
* **01-04:** wire atomic state and runtime deps ([0e6d926](https://github.com/aaronflorey/docker-dns-sync/commit/0e6d926214b0112bc215788b69ebb6cb9e241e5a))
* **02-01:** implement ownership-safe reconcile planner and apply flow ([ba43924](https://github.com/aaronflorey/docker-dns-sync/commit/ba43924da2d3f6e16af16eede69685bbde5ab49c))
* **02-02:** implement real AdGuard output transport ([59e03f0](https://github.com/aaronflorey/docker-dns-sync/commit/59e03f0c529297b35744f76580a4ae9dcd108c62))
* **03-04:** complete docker sync automation and runtime recovery ([7cb50d3](https://github.com/aaronflorey/docker-dns-sync/commit/7cb50d3a35b0f7a1b39075de489f6a665a7ef6d8))
* add live Docker smoke test workflow ([79e5699](https://github.com/aaronflorey/docker-dns-sync/commit/79e5699861db0e0108619ed6ba039c4ebe7cd01b))
* add provenance-gated stale cleanup and operation timeouts ([6e2dad6](https://github.com/aaronflorey/docker-dns-sync/commit/6e2dad67b69ad685bf522e4c62d4c481808be3df))
* **config:** add source host ip support ([e52b4db](https://github.com/aaronflorey/docker-dns-sync/commit/e52b4db9808fd7b59d6a2592e52464bc007a2f3a))
* debounce watch hints and add safe derivation diagnostics ([74c123a](https://github.com/aaronflorey/docker-dns-sync/commit/74c123aa692a1c21d78fc8c47056de0b98544331))
* fix logging ([57653c4](https://github.com/aaronflorey/docker-dns-sync/commit/57653c41e712c0f6dfaf0eb4895cdd4a1c1e7264))


### Bug Fixes

* **02-01:** run startup reconciliation in runtime ([56a029d](https://github.com/aaronflorey/docker-dns-sync/commit/56a029d744718296b89a6bf2c8a0bb13a79b7030))
* **05:** close milestone audit gaps ([969a122](https://github.com/aaronflorey/docker-dns-sync/commit/969a122c9366babd3ef2bd63a7970f4bf25089da))
* **cloudflare:** recover duplicate create conflicts ([9a2bc9b](https://github.com/aaronflorey/docker-dns-sync/commit/9a2bc9b1b6917a0fe882f466c6d63da5ae48b818))
* **cloudflare:** take over existing single-host records ([c11f292](https://github.com/aaronflorey/docker-dns-sync/commit/c11f2924b6996c6d263ed253c27d86246bc76e1e))
* **config:** add docker base domain support ([9da24d0](https://github.com/aaronflorey/docker-dns-sync/commit/9da24d0f7646322f60c585de0236b1a5da3bfe71))
* **docker:** add proxy.dns output targeting ([fa2787e](https://github.com/aaronflorey/docker-dns-sync/commit/fa2787edc897b04740268404879138b385998e95))
* reject unknown config keys during load ([7dbe3b2](https://github.com/aaronflorey/docker-dns-sync/commit/7dbe3b21e7e118b626ad4df11ecf443ed4e8b9bd))
* **release:** migrate goreleaser config ([f7e5292](https://github.com/aaronflorey/docker-dns-sync/commit/f7e5292d93dc770c71f73a93937ea855b5e8d51c))
* resolve post-merge conflicts from wave 1 ([ab654d5](https://github.com/aaronflorey/docker-dns-sync/commit/ab654d5306b94a0ebcef6228ed942bdf2529c043))
* **runtime:** add trace logging and fix cloudflare visible matching ([7b2b2e9](https://github.com/aaronflorey/docker-dns-sync/commit/7b2b2e945864cec2d601285504ee617feb8cda51))
* **runtime:** recover owned same-host drift ([1cde591](https://github.com/aaronflorey/docker-dns-sync/commit/1cde5915853846ca0e6cf3746bcb7460026cc353))
* **runtime:** retry transient source reads ([24d71f8](https://github.com/aaronflorey/docker-dns-sync/commit/24d71f8ae1da8a6d026f6910de83f389715be6db))
* use remote provenance for Cloudflare record mutations ([778168e](https://github.com/aaronflorey/docker-dns-sync/commit/778168e4eb0c4c3125473b660e973d0af10fe17f))

## [0.2.1](https://github.com/aaronflorey/docker-dns-sync/compare/v0.2.0...v0.2.1) (2026-06-21)


### Bug Fixes

* **docker:** add proxy.dns output targeting ([1ac8811](https://github.com/aaronflorey/docker-dns-sync/commit/1ac88110ea063467bd7bde8ad5c1cb45faf194ba))

## [0.2.0](https://github.com/aaronflorey/docker-dns-sync/compare/v0.1.0...v0.2.0) (2026-05-28)


### Features

* **01-01:** implement config-driven startup and cancellable runtime ([cf45aca](https://github.com/aaronflorey/docker-dns-sync/commit/cf45acabc2451709fb7ae09a956e3724c0ffb287))
* **01-02:** add config validation and secret resolution ([45b0e6a](https://github.com/aaronflorey/docker-dns-sync/commit/45b0e6ac04e74351d80a6a10be5adbb2756e2c3b))
* **01-03:** add provider contracts and runtime factories ([0fd7d6f](https://github.com/aaronflorey/docker-dns-sync/commit/0fd7d6f1dc2419f64f545ba513b172d8bc674271))
* **01-04:** wire atomic state and runtime deps ([4925df8](https://github.com/aaronflorey/docker-dns-sync/commit/4925df847573a3f04bb9534173c3edd44b6738e0))
* **02-01:** implement ownership-safe reconcile planner and apply flow ([316c220](https://github.com/aaronflorey/docker-dns-sync/commit/316c220ed345874bf7067b6ed90d467a7cf78ec3))
* **02-02:** implement real AdGuard output transport ([f7151c5](https://github.com/aaronflorey/docker-dns-sync/commit/f7151c538ce486cba6f3ae3f834f107cfb647dbb))
* **03-04:** complete docker sync automation and runtime recovery ([9924c4f](https://github.com/aaronflorey/docker-dns-sync/commit/9924c4f49f527aa7ec237bd051ab9c5710dfd473))
* **config:** add source host ip support ([5f8f944](https://github.com/aaronflorey/docker-dns-sync/commit/5f8f944027f8658560a8053c7dc8d4b8ce90bf8d))
* fix logging ([8077791](https://github.com/aaronflorey/docker-dns-sync/commit/80777911b171ae56e855fc01b6f5563cede75625))


### Bug Fixes

* **02-01:** run startup reconciliation in runtime ([bbaf584](https://github.com/aaronflorey/docker-dns-sync/commit/bbaf5849e68d8888e0e14b19eb32c2510d0294ec))
* **05:** close milestone audit gaps ([2858dd7](https://github.com/aaronflorey/docker-dns-sync/commit/2858dd78debd56f12a015cdd491de54420fdd9e4))
* **cloudflare:** recover duplicate create conflicts ([4046e4d](https://github.com/aaronflorey/docker-dns-sync/commit/4046e4d0b9ff31e7f74bae26b95a4aea59151318))
* **cloudflare:** take over existing single-host records ([8d5af94](https://github.com/aaronflorey/docker-dns-sync/commit/8d5af946b088229e7a83b82c4a50cdbf046dd05c))
* **config:** add docker base domain support ([b9b1d56](https://github.com/aaronflorey/docker-dns-sync/commit/b9b1d567954e38fe696d87797bd015083345d324))
* **release:** migrate goreleaser config ([4c47f2b](https://github.com/aaronflorey/docker-dns-sync/commit/4c47f2b75ec00fc0221435dd4f9faf28948271fc))
* resolve post-merge conflicts from wave 1 ([712477b](https://github.com/aaronflorey/docker-dns-sync/commit/712477b85c9b68cbf9ba28334d8d793d4c739c47))
* **runtime:** add trace logging and fix cloudflare visible matching ([22df5da](https://github.com/aaronflorey/docker-dns-sync/commit/22df5daa1afc88275798d46043dd62d3d06bad91))
* **runtime:** recover owned same-host drift ([5242731](https://github.com/aaronflorey/docker-dns-sync/commit/52427318242eddd5babd9c0e9b04dfaba13c5a6f))
* **runtime:** retry transient source reads ([ab47fbd](https://github.com/aaronflorey/docker-dns-sync/commit/ab47fbdbf170ebe3395daf6e2672573463b58ebb))
