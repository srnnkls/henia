# Changelog

## [0.1.0-alpha.12](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.11...v0.1.0-alpha.12) (2026-10-04)


### Features

* **runtime:** sandbox Linux preloads with Landlock ([4c0dc60](https://github.com/srnnkls/henia/commit/4c0dc60ae628c312adea58dd49a2b80828c4038e))


### Bug Fixes

* **runtime:** run only a skill's declared preloads through henia preload ([e440c0c](https://github.com/srnnkls/henia/commit/e440c0cba04f1861b4743ee14b8849befbd6e6b4))

## [0.1.0-alpha.11](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.10...v0.1.0-alpha.11) (2026-10-04)


### Features

* **config:** name static and dynamic skills and clarify harness keys ([ca04276](https://github.com/srnnkls/henia/commit/ca042763dc373f3240ef22b139abf87df194fafc))
* **runtime:** configure the FAS consultation timeout ([13bddac](https://github.com/srnnkls/henia/commit/13bddac9ea0d4f91225d041ff5a7903d12572181))

## [0.1.0-alpha.10](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.9...v0.1.0-alpha.10) (2026-10-03)


### Features

* **build:** route projected preloads through henia preload ([170bf03](https://github.com/srnnkls/henia/commit/170bf0343a5fd8b7e6527e2b8873cb891f14b3b8))
* **runtime:** run skill preloads in henia show ([3976dec](https://github.com/srnnkls/henia/commit/3976decb757d5f3b5c8c72fd3910ac9171f130dd))

## [0.1.0-alpha.9](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.8...v0.1.0-alpha.9) (2026-10-03)


### Bug Fixes

* **runtime:** honour a harness's exclude list when rendering references ([5151b93](https://github.com/srnnkls/henia/commit/5151b934c53842af843044df22c01ee82691145d))
* **runtime:** keep a skill's references to itself in harness syntax ([6322649](https://github.com/srnnkls/henia/commit/632264971ce996c74395783e26ac1b9dfa1cf1d5))
* **runtime:** reference skills the model may not invoke through henia show ([a94d7f1](https://github.com/srnnkls/henia/commit/a94d7f118e84ca7f5abf9d008a85ff5e82cfad34))

## [0.1.0-alpha.8](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.7...v0.1.0-alpha.8) (2026-10-03)


### Features

* **runtime:** serve library skills through henia ls, show and context ([9201691](https://github.com/srnnkls/henia/commit/9201691845b5c8f7c126a3e79d3be8b3c9ba9c6a))


### Bug Fixes

* **runtime:** render for the nearest calling agent process ([1f8bd0f](https://github.com/srnnkls/henia/commit/1f8bd0f9fa4c241bedccd6485a123de95025e32b))
* **slots:** point library providers at henia show, not their files ([06a3cc2](https://github.com/srnnkls/henia/commit/06a3cc288cd1ea452b36a9bf51ae291bacf25c5c))

## [0.1.0-alpha.7](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.6...v0.1.0-alpha.7) (2026-10-03)


### Features

* **slots:** model consumers with metadata.applies and resolve them with --for ([94dd8ca](https://github.com/srnnkls/henia/commit/94dd8ca8ab86bd0d2060fe1f172ee375b27199e9))
* **slots:** type slot values as commands, text or paths ([15f2560](https://github.com/srnnkls/henia/commit/15f25602a8c6e97fd37ff9232e80e8acf94b9ce3))

## [0.1.0-alpha.6](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.5...v0.1.0-alpha.6) (2026-10-03)


### Features

* **slots:** resolve typed skill slots at runtime ([bec2547](https://github.com/srnnkls/henia/commit/bec25472d1ed3e5ee984280a63c86b91d615308d))

## [0.1.0-alpha.5](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.4...v0.1.0-alpha.5) (2026-10-02)


### Features

* **lint:** check cross-document names with registries ([6324fc4](https://github.com/srnnkls/henia/commit/6324fc4e0d847fb0407276f741efaec4fe1f8a96))

## [0.1.0-alpha.4](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.3...v0.1.0-alpha.4) (2026-09-30)


### Features

* render references in support files and declare external lint names ([#7](https://github.com/srnnkls/henia/issues/7)) ([20a45b7](https://github.com/srnnkls/henia/commit/20a45b7af36f8ea08ea7c94abb72ea2eaa4bf9f8))

## [0.1.0-alpha.3](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.2...v0.1.0-alpha.3) (2026-09-30)


### Features

* **lint:** skip Git-ignored paths ([#5](https://github.com/srnnkls/henia/issues/5)) ([b6cd928](https://github.com/srnnkls/henia/commit/b6cd928018111ab0ead517fe3ccdab550803d3b5))

## [0.1.0-alpha.2](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.1...v0.1.0-alpha.2) (2026-09-30)


### Bug Fixes

* **lint:** skip empty headings instead of panicking ([c14cafb](https://github.com/srnnkls/henia/commit/c14cafba582fafd87dd67bf5db407b8d36fc44c3))
