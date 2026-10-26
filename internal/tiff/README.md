# TIFF decoder used by OShell Desktop

This package copies `golang.org/x/image/tiff` at v0.46.0 under its BSD license,
including its reader/writer regression tests and fixtures. `upstream.json`
records the original files and SHA-256 values for future upgrades. The original
CCITT and TIFF LZW packages remain dependencies of the pinned x/image module.
The small internal `safemath` helper is copied because Go internal-package rules
prevent importing it here. Test fixture paths and import paths are adjusted.

Desktop adds grayscale plus alpha decoding at 8 and 16 bits per
sample. Both WhiteIsZero and BlackIsZero, associated and unassociated alpha,
strips, padded tiles, and the upstream decompression/predictor paths are used.
Associated inverse gray is inverted within the alpha range; zero alpha has zero
associated color. Separate planes for grayscale alpha, RGB and RGBA use the same
decompression and prediction paths. Offset/count tables include every plane;
one block is combined at a time before pixel decoding. Physical block order is
independent of its position in the offset table. Unequal sample depths and
unspecified extra channels remain unsupported.

The PNG clipboard conversion retains gray ICC profiles through color-type 4
output. Terminal image decoders import this same package. No second TIFF image
registration should be imported from the original package in application tests.

The added `testfixture` package is only referenced by tests. It generates
synthetic samples and independent byte streams; it contains no user images.
Existing upstream fixtures and tests remain to check previously supported
formats. This does not add every TIFF extension or prove ImageIO equivalence.

Reference: [TIFF Revision 6.0, ExtraSamples and associated alpha](https://image-js.github.io/tiff/media/TIFF6.pdf), sections 8 and 21.
