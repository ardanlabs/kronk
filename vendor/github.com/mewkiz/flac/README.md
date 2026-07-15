# flac

[![Go build status](https://github.com/mewkiz/flac/actions/workflows/go.yml/badge.svg?branch=master)](https://github.com/mewkiz/flac/actions/workflows/go.yml)
[![Coverage Status](https://coveralls.io/repos/github/mewkiz/flac/badge.svg?branch=master)](https://coveralls.io/github/mewkiz/flac?branch=master)
[![GoDoc](https://pkg.go.dev/badge/github.com/mewkiz/flac)](https://pkg.go.dev/github.com/mewkiz/flac)

This package provides access to [FLAC][1] (Free Lossless Audio Codec) streams.

[1]: http://flac.sourceforge.net/format.html

## Documentation

Documentation provided by GoDoc.

- [flac]: provides access to FLAC (Free Lossless Audio Codec) streams.
    - [frame][flac/frame]: implements access to FLAC audio frames.
    - [meta][flac/meta]: implements access to FLAC metadata blocks.

[flac]: http://pkg.go.dev/github.com/mewkiz/flac
[flac/frame]: http://pkg.go.dev/github.com/mewkiz/flac/frame
[flac/meta]: http://pkg.go.dev/github.com/mewkiz/flac/meta

## Changes

* Version 1.0.13 (2025-07-11)
    - Add support for Fixed prediction encoding of FLAC subframes (see [#76](https://github.com/mewkiz/flac/pull/76)). Thanks to [Michael Braha](https://github.com/braheezy).

* Version 1.0.12 (2024-08-11)
    - Improve performance of `flac.NewSeek()` by using a buffered reader (see [#72](https://github.com/mewkiz/flac/pull/72)).
    - Fix off-by-one error in `Seek` end of stream check (see [#73](https://github.com/mewkiz/flac/pull/73)).

* Version 1.0.11 (2024-08-04)
    - Move example tools to dedicated [mewkiz/flac-tools](https://github.com/mewkiz/flac-tools) repository to reduce external dependencies (see [#62](https://github.com/mewkiz/flac/pull/62)).
    - Fix seek to frame start (see [#71](https://github.com/mewkiz/flac/pull/71)). Thanks to [Mark Kremer](https://github.com/MarkKremer).
    - Simplify internal handling of `io.Closer` element in `flac.Stream` and `flac.Encoder` types (see [#70](https://github.com/mewkiz/flac/pull/70)). Thanks to [Mario Salgado](https://github.com/zalgonoise).

* Version 1.0.10 (2023-11-11)
    - Add support for LPC audio sample encoding (see [#66](https://github.com/mewkiz/flac/pull/66)). Thanks to [Mark Kremer](https://github.com/MarkKremer) for bug fixes and [Mattias Wadman](https://github.com/wader) for the invaluable [fq](https://github.com/wader/fq) tool used to investigate FLAC encoding issues.
    - Replace Travis CI with GitHub actions for CI build status, test status and code coverage [#64](https://github.com/mewkiz/flac/pull/64)). Thanks to [Mark Kremer](https://github.com/MarkKremer).

* Version 1.0.9 (2023-10-24)
    - Fix integer overflow during unfolding of rice residual (see [#61](https://github.com/mewkiz/flac/pull/61)). Thanks to [Mark Kremer](https://github.com/MarkKremer).
    - Fix decoding of escaped partition audio samples (see [#60](https://github.com/mewkiz/flac/issues/60)). Thanks to [Mark Kremer](https://github.com/MarkKremer).
    - Handle frame hashing of audio samples with bits-per-sample not evenly divisible by 8 (see [9d50c9e](https://github.com/mewkiz/flac/commit/9d50c9ee99ba322f487ed60442dc16f22b2affb8)).

* Version 1.0.8 (2023-04-09)
    - Fix race condition when reading meta data (see [#56](https://github.com/mewkiz/flac/pull/56)). Thanks to [Zach Orosz](https://github.com/zachorosz).
    - Fix encoding of 8-bps WAV audio samples (see [#52](https://github.com/mewkiz/flac/pull/52)). Thanks to [Martijn van Beurden](https://github.com/ktmf01).
    - Fix `StreamInfo` block type error message (see [#49](https://github.com/mewkiz/flac/pull/49)).

* Version 1.0.7 (2021-01-28)
    - Add seek API (see [#44](https://github.com/mewkiz/flac/pull/44) and [#46](https://github.com/mewkiz/flac/pull/46)). Thanks to [Craig Swank](https://github.com/cswank).

* Version 1.0.6 (2019-12-20)
    - Add experimental Encoder API to encode audio samples and metadata blocks (see [#32](https://github.com/mewkiz/flac/pull/32)).
    - Use go.mod.
    - Skip ID3v2 data prepended to flac files when parsing (see [36cc17e](https://github.com/mewkiz/flac/commit/36cc17efed51a9bae283d6a3a7a10997492945e7)).
        - Remove dependency on encodebytes. Thanks to [Mikey Dickerson](https://github.com/mdickers47).
    - Add 16kHz test case. Thanks to [Chewxy](https://github.com/chewxy).
    - Fix lint issues (see [#25](https://github.com/mewkiz/flac/issues/25)).

* Version 1.0.5 (2016-05-06)
    - Simplify import paths. Drop use of gopkg.in, and rely on vendoring instead (see [azul3d/engine#1](https://github.com/azul3d/engine/issues/1)).
    - Add FLAC decoding benchmark (see [d675e0a](https://github.com/mewkiz/flac/blob/d675e0aaccf2e43055f56b9b3feeddfdeed402e2/frame/frame_test.go#L60)).

* Version 1.0.4 (2016-02-11)
    - Add API examples to documentation (see [#11](https://github.com/mewkiz/flac/issues/11)).
    - Extend test cases (see [aadf80a](https://github.com/mewkiz/flac/commit/aadf80aa28c463a94b8d5c49757e5a0948613ce2)).

* Version 1.0.3 (2016-02-02)
    - Implement decoding of FLAC files with wasted bits-per-sample (see [#12](https://github.com/mewkiz/flac/issues/12)).
    - Stress test the library using [go-fuzz](https://github.com/dvyukov/go-fuzz) (see [#10](https://github.com/mewkiz/flac/pull/10)). Thanks to [Patrick Mézard](https://github.com/pmezard).

* Version 1.0.2 (2015-06-05)
    - Fix decoding of blocking strategy (see [#9](https://github.com/mewkiz/flac/pull/9)). Thanks to [Sergey Didyk](https://github.com/sdidyk).

* Version 1.0.1 (2015-02-25)
    - Fix two subframe decoding bugs (see [#7](https://github.com/mewkiz/flac/pull/7)). Thanks to [Jonathan MacMillan](https://github.com/perotinus).
    - Add frame decoding test cases.

* Version 1.0.0 (2014-09-30)
    - Initial release.
    - Implement decoding of FLAC files.
