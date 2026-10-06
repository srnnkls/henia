# Changelog

## [0.1.0-alpha.18](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.17...v0.1.0-alpha.18) (2026-10-06)


### ⚠ BREAKING CHANGES

* **query:** a capture inside a pattern's parentheses no longer names the enclosing node; (skill @s ...) is an error, and (skill :id "git" @s) captures "git". Aggregate outputs and scores take @name instead of ?name, lint rules take :score @score, and --sort names them as @n.
* **preload:** Henia no longer runs a policy program it finds on PATH, and preload.fas_timeout is now preload.policy_timeout.

### Features

* **preload:** ask only a policy command the user configures ([135e96c](https://github.com/srnnkls/henia/commit/135e96c7f9325b8ef4bfdca0fb14d3adc0593fa9))
* **query:** a capture applies to what precedes it ([793ca49](https://github.com/srnnkls/henia/commit/793ca49550c7fc4a050e67a9ff7a728b745cdd52))
* **query:** accept top-level optional patterns as left joins ([ba9cab7](https://github.com/srnnkls/henia/commit/ba9cab7d596ad6195b487e0057082f4f0a4de188))
* **query:** count words, chars and lines on skill and file nodes ([3fa844f](https://github.com/srnnkls/henia/commit/3fa844f89d1a1c1fe4d7ea252bc43b84d5bd5109))
* **query:** fall back per enclosing row for optional siblings and relation targets ([d3b2ea9](https://github.com/srnnkls/henia/commit/d3b2ea92d861eb1b1393d6310a81c72a50570455))
* **query:** group rows and aggregate them with (group ...) ([6ca5cd9](https://github.com/srnnkls/henia/commit/6ca5cd94cc264ae9dc15df41b86eb890831a172a))
* **query:** let optional bindings join a required binder matched before them ([b54b05c](https://github.com/srnnkls/henia/commit/b54b05cb72396ed653354dc152c54cbef09184b1))
* **query:** sort rows with --sort ([e37b6a8](https://github.com/srnnkls/henia/commit/e37b6a860acfbcbc9683b57d2a48c20da54e1c5e))


### Bug Fixes

* **library:** shadow a global package named like a project dependency ([2f23a99](https://github.com/srnnkls/henia/commit/2f23a99f687103bdc82399181aa61f89eebfcdb6))
* **library:** strip frontmatter the parser recognizes when it fails to decode ([2a32d7f](https://github.com/srnnkls/henia/commit/2a32d7fac73c369a59173cc78b4a3010fe65bb05))
* **profile:** move vendor profiles out of a vendor directory ([542ced0](https://github.com/srnnkls/henia/commit/542ced0b7607547ab462a2a2e64e41e4d6c778da))
* **reference:** a # reference names a file only when it looks like a path ([b1998ae](https://github.com/srnnkls/henia/commit/b1998aeea3be06829ea5561af3eb8e41911dae93))
* **transform:** render undefined template variables as nothing ([3c70ab9](https://github.com/srnnkls/henia/commit/3c70ab92d653282a764ef64bbed0725ad2f1c036))


### Performance Improvements

* **library:** compute skill digests and sections only for ls --json ([52e2c70](https://github.com/srnnkls/henia/commit/52e2c702a21ee7ebc4e8202f69c5b4fc4ae9877b))
* **markup:** share goldmark parsers and count parses ([690799b](https://github.com/srnnkls/henia/commit/690799b20f4de12109a58c663352c221608d09ee))
* **query:** match typed patterns against a node-kind index ([28457d8](https://github.com/srnnkls/henia/commit/28457d8c535b58c2721b107955342b1132161381))
* **transform:** cache reference templates and splice links in one pass ([c1c887f](https://github.com/srnnkls/henia/commit/c1c887fe0c6ef7ee0180b474b76dbcbde8a61d9e))

## [0.1.0-alpha.17](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.16...v0.1.0-alpha.17) (2026-10-05)


### ⚠ BREAKING CHANGES

* **show:** detect stale hybrid heads without a digest argument

### Features

* **show:** address resources like modules ([8fbb3c0](https://github.com/srnnkls/henia/commit/8fbb3c0f18b3a886b134c43f55927792063d4f93))


### Bug Fixes

* **render:** rewrite local links to henia show addresses where files are absent ([c2dfd90](https://github.com/srnnkls/henia/commit/c2dfd90e15860b97ca21294903d8a00e3222c4bf))
* **show:** detect stale hybrid heads without a digest argument ([050e25c](https://github.com/srnnkls/henia/commit/050e25cf6ed6ce83d3bff1052b373f605910df0c))
* **slots:** report requested slots without providers and empty context ([aa2d233](https://github.com/srnnkls/henia/commit/aa2d2334690fd3b9de3f768eacf36400cddf98a6))

## [0.1.0-alpha.16](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.15...v0.1.0-alpha.16) (2026-10-05)


### Features

* **install:** infer harness homes; enabled = false uninstalls ([67751ba](https://github.com/srnnkls/henia/commit/67751ba57c1b2c41740c9f95e4d90fe4023705e2))


### Bug Fixes

* **install:** render the henia skill with the owning package's harness profile ([ad956e0](https://github.com/srnnkls/henia/commit/ad956e0153c95f7aff2b8fe79e0806086a27f031))

## [0.1.0-alpha.15](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.14...v0.1.0-alpha.15) (2026-10-05)


### ⚠ BREAKING CHANGES

* **build:** static, dynamic and hybrid skill modes
* **lint:** lint package contents, not every Markdown file

### Features

* **build:** static, dynamic and hybrid skill modes ([39daba2](https://github.com/srnnkls/henia/commit/39daba2dc7fbdb457d6076a0520255b4c1043e56))
* **deps:** henia.local.toml overrides and live-linked path dependencies ([dc89dea](https://github.com/srnnkls/henia/commit/dc89deabe11bfe164b4cad11e880f666bdbfb479))
* **install:** install henia's own skill in every harness ([6b09210](https://github.com/srnnkls/henia/commit/6b09210726895cc6178d9b0b8fb7a5e697cbf934))
* **install:** write global skill packages into harness directories ([79fff44](https://github.com/srnnkls/henia/commit/79fff44537f4b1d8e5736ae1efe39e6b14e72ca4))
* **lint:** lint package contents, not every Markdown file ([ea1f357](https://github.com/srnnkls/henia/commit/ea1f35763dfb62c66927cbbc1ce75039a40b63f6))


### Bug Fixes

* **deps:** no lock without declared packages ([8b84846](https://github.com/srnnkls/henia/commit/8b84846dfddb002f313852f474ed5f9084457a1c))
* **slots:** lookups report only problems of the requested slots ([3e6f6fb](https://github.com/srnnkls/henia/commit/3e6f6fbebd14c186bdb9db559dbb0299c8b2876e))

## [0.1.0-alpha.14](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.13...v0.1.0-alpha.14) (2026-10-05)


### ⚠ BREAKING CHANGES

* **slots:** compose skills over the library

### Features

* **slots:** compose skills over the library ([b704da7](https://github.com/srnnkls/henia/commit/b704da75e135ebb1e3e6d4cabcf66d26834a161b))


### Bug Fixes

* **slots:** skip templated directives; read symlinked global packages ([39fc93f](https://github.com/srnnkls/henia/commit/39fc93f0dc03af2e3b0687a76472611b0364bfd5))

## [0.1.0-alpha.13](https://github.com/srnnkls/henia/compare/v0.1.0-alpha.12...v0.1.0-alpha.13) (2026-10-05)


### Features

* build and lint sync declared packages missing from the store ([80d9943](https://github.com/srnnkls/henia/commit/80d9943e23a50b603de0f3964952102956b87945))
* **build:** dependency harness settings as overridable defaults ([0e2babf](https://github.com/srnnkls/henia/commit/0e2babfa436ee9246bc7b5b21704f3f67e3306a8))
* **config:** remove legacy config fallbacks ([7d4767b](https://github.com/srnnkls/henia/commit/7d4767bcabbb25e39c0968ef223f1c0d12d70679))
* **deps:** fetch the pinned phora release when none fits ([6d71e04](https://github.com/srnnkls/henia/commit/6d71e0455c02d5c7b655f26419c9bbde5b173ce4))
* henia add/rm/sync/update manage skill packages through phora ([3a30b06](https://github.com/srnnkls/henia/commit/3a30b06d4316e0212776d9ffb8cdabddc152690f))
* **library:** corpus facts for lint and a corpus from scanned documents ([4fe11de](https://github.com/srnnkls/henia/commit/4fe11de7998987e6c585c9bb791b71a9bf6af8b0))
* **library:** dependency sources under .henia/sources ([d02363b](https://github.com/srnnkls/henia/commit/d02363bd2e2b188e84cdd642ce3ea9850e75b302))
* **lint:** check henia show section references ([b970482](https://github.com/srnnkls/henia/commit/b970482ce83ed3ee413ec02fa54b10b216768a54))
* **lint:** compile lint configuration into a typed plan ([18aece6](https://github.com/srnnkls/henia/commit/18aece65720dae1c7a7863eab31cb17e91566ad2))
* **lint:** express metadata, reference and duplicate rules in the standard library ([bb00cad](https://github.com/srnnkls/henia/commit/bb00cad8198b7f355d34c43733a3164bbb6bf6f9))
* **lint:** load lint modules from sources, user and project, and test them ([737ba70](https://github.com/srnnkls/henia/commit/737ba7067f3169fef83eff425220e858c0bd2708))
* **lint:** resolve references against dependencies, the library and harness built-ins ([240cd52](https://github.com/srnnkls/henia/commit/240cd52f87906d33046c0ca89e2263393101d2ff))
* **lint:** run lint rules from hq modules with a standard library ([f0bce81](https://github.com/srnnkls/henia/commit/f0bce81aeeceab3f3cf831eff09b053357ab4207))
* **lint:** similarity rules in the standard library ([76ee417](https://github.com/srnnkls/henia/commit/76ee417ef8a2c11ef423ce310a7e3a1b6791182e))
* **lint:** slot rules in the standard library; registries give way to hq ([6653301](https://github.com/srnnkls/henia/commit/6653301c2caa728ac9ce491bb23deabd70a0fd05))
* **query:** join patterns on shared variables ([5f7f63a](https://github.com/srnnkls/henia/commit/5f7f63ac38d8a6570baaac04409e2fb22c452924))
* **query:** join top-level patterns on shared variables and follow links ([e271f09](https://github.com/srnnkls/henia/commit/e271f09612d08aeeae87b3ee9073a10bf9103e8f))
* **query:** match inbound skill links and negate relations ([9e164d3](https://github.com/srnnkls/henia/commit/9e164d305ed82b8f7e7412422eeda215845348dc))
* **query:** measure words and compare text by shingles and embeddings ([1503939](https://github.com/srnnkls/henia/commit/1503939c4414c689a60f0a9f4106294b958ff9ef))
* **query:** params, comparisons, data rows and date checks ([db70f38](https://github.com/srnnkls/henia/commit/db70f3802d79126b2b9eaf30fa2145d2c36bd152))
* **query:** query skills and resources with S-expression patterns ([4eb0507](https://github.com/srnnkls/henia/commit/4eb0507007c10d78ff440f080c8bde92f696b739))
* **query:** read modules of rules, defines and imports ([362e74a](https://github.com/srnnkls/henia/commit/362e74ac53ac575991cbb38c374caa9770c64f7e))
* **runtime:** disclose a skill's resources in henia show ([5f5a5ce](https://github.com/srnnkls/henia/commit/5f5a5cec8e7d702f505c62ba56313d93bdd5931c))


### Bug Fixes

* **lint:** dependency lint modules live in the source's .henia/lint ([c3b07ad](https://github.com/srnnkls/henia/commit/c3b07ad3d45e9755a543326c78759d767fa20a6c))
* **lint:** read only artifacts from git-ignored dependencies ([c0fea9b](https://github.com/srnnkls/henia/commit/c0fea9bb6732e9a14ac18ec0f6b73f21c541f30a))

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
