# Third-party notices

OShell Desktop original code and OShell artwork: GPL-3.0-only, see LICENSE.

Dependencies retain their own licenses. Exact module versions are in go.mod / go.sum.

| Component | License | Source |
| --- | --- | --- |
| mygo (local runtime renderer-selection patch) | MIT | https://github.com/egoist/mygo |
| libghostty-vt | MIT | https://github.com/ghostty-org/ghostty |
| SwiftTerm Lab palette, inline-image sizing and Sixel behavior (adapted from OShell's vendored version) | MIT | https://github.com/oshell-desktop/oshell/tree/main/Vendor/SwiftTerm |
| purego | Apache-2.0 | https://github.com/ebitengine/purego |
| go-text/typesetting | BSD-3-Clause / Unlicense | https://github.com/go-text/typesetting |
| clipperhouse/uax29 v2.7.0 (tab-title grapheme segmentation) | MIT | https://github.com/clipperhouse/uax29 |
| golang.org/x/crypto, x/sys, x/image | BSD-3-Clause | https://go.googlesource.com/ |
| pkg/sftp | BSD-2-Clause | https://github.com/pkg/sftp |
| jlaffaye/ftp | ISC | https://github.com/jlaffaye/ftp |
| gen2brain/h265 (HEIC decoder) | MIT | https://github.com/gen2brain/h265 |
| Microsoft/go-winio | MIT | https://github.com/microsoft/go-winio |
| xfer ZMODEM implementation | WTFPL (upstream README) | https://github.com/arttu76/xfer |
| Cascadia Code (Regular, Bold, Italic, Bold Italic) | SIL OFL 1.1 | https://github.com/microsoft/cascadia-code |
| DejaVu Sans Mono | Bitstream Vera / public domain additions | https://dejavu-fonts.github.io/ |
| Symbols Nerd Font | MIT and component licenses | https://github.com/ryanoasis/nerd-fonts |
| kr/fs | BSD-3-Clause | https://github.com/kr/fs |

Development fallback library hashes are pinned by internal/terminal/mygo-plugin.json. Windows/Linux packages build the pinned Ghostty source with third_party/ghostty/oshell.patch; scripts/terminal_runtime.py verifies the compiler and caches the resulting SHA-256. Ghostty theme data and fonts may carry separate upstream notices. License texts for the shipped components are included under resources/licenses.

## Additional licenses and data

- regexp2 v1.12.0 (MIT): resources/licenses/regexp2-LICENSE.
- gen2brain/h265 v0.2.3 (MIT): resources/licenses/h265-LICENSE. The HEIC regression samples in internal/core/testdata/heic come from the same version.
- golang.org/x/text v0.42.0 and golang.org/x/term v0.46.0 (BSD): corresponding resources/licenses files.
- `internal/tiff` copies golang.org/x/image/tiff v0.46.0 and its safemath helper, tests and fixtures (BSD-3-Clause; local LICENSE and resources/licenses/x-image-LICENSE). Local changes add grayscale alpha and planar gray/RGB(A) decoding; original file hashes and scope are recorded in internal/tiff/upstream.json and README.md.
- Unicode 17.0.0 East Asian Width ranges (Unicode-3.0): resources/licenses/Unicode-LICENSE.
- The terminal BiDi core under `internal/terminal/internal/bidi` derives from go-text/typesetting v0.3.5 (BSD-3-Clause / Unlicense; local LICENSE), with row-level L1/L2 processing. Cell presentation and Unicode tables under `bidilayout` derive from SwiftTerm (MIT; resources/licenses/SwiftTerm-LICENSE) and Unicode 17.0.0 (Unicode-3.0).
- Windows / Linux packages include OShell's ambiguous-width, synchronous color-palette, terminal-reset, inline-image and Sixel extensions, built from the pinned Ghostty commit and Unicode 17.0.0 ranges in third_party/ghostty. Original Ghostty licensing remains applicable.

## FTP listing metadata patch

`third_party/ftp` contains github.com/jlaffaye/ftp v0.2.4 (ISC; see its LICENSE). Local changes retain UNIX LIST and MLSD permissions and ownership, recognize MLSD symbolic links, and provide strict MLSD-to-LIST fallback so malformed rows cannot silently disappear.

The OShell listing path also limits MLSD/LIST/NLST responses to 4 MiB and treats size/UTF-8 violations as terminal errors. Strict LIST parsing retains UTC timestamps from RFC3659 facts; legacy ls/DIR dates without an explicit zone (or year) are displayed as unknown rather than inferred.

`third_party/sftp` contains github.com/pkg/sftp v1.13.11 (BSD-2-Clause; see LICENSE), patched to retain verified UNIX long-listing owner/group display hints. Numeric SFTP attributes remain authoritative.

## Fixed legacy SSH algorithms (SF-C03)

`third_party/xcrypto` contains golang.org/x/crypto v0.57.0 (BSD-3-Clause;
original hashes in upstream.json). The local SSH patch registers AES192-CBC and
AES256-CBC using the existing AES-CBC implementation, outside defaults/FIPS.
It also corrects the CBC path for already-negotiated EtM MACs: clear length,
MAC over ciphertext, authentication before decryption. Non-EtM oracle
mitigation remains unchanged. See third_party/xcrypto/OSHELL_PATCH.md.
