HEIC regression samples: basic.heic, alpha.heic, main10.heic.
Source: github.com/gen2brain/h265 v0.2.3, heic/testdata.
License: MIT; full text in resources/licenses/h265-LICENSE.
Upstream checks the decoded planes against libde265 reference digests.
These are synthetic test images, not user clipboard content.
The upstream main10.heic sample has 8-bit coded pixels; the explicit 10-bit
regression generates its own lossless image and checks sub-8-bit precision.
