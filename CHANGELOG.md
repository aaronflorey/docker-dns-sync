# Changelog

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
