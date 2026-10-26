module oshell-desktop

go 1.27.1

require (
	github.com/Microsoft/go-winio v0.6.2
	github.com/clipperhouse/uax29/v2 v2.7.0
	github.com/dlclark/regexp2 v1.12.0
	github.com/ebitengine/purego v0.11.1
	github.com/egoist/mygo v0.3.5-0.20261008155018-bad73d119dc8
	github.com/gen2brain/h265 v0.2.3
	github.com/go-text/typesetting v0.3.5
	github.com/jlaffaye/ftp v0.2.4
	github.com/pkg/sftp v1.13.11
	golang.org/x/crypto v0.57.0
	golang.org/x/image v0.46.0
	golang.org/x/sys v0.48.0
	golang.org/x/text v0.42.0
)

require github.com/kr/fs v0.1.0 // indirect

tool github.com/egoist/mygo/cmd/mygo

replace github.com/egoist/mygo => ./third_party/mygo

replace github.com/jlaffaye/ftp => ./third_party/ftp

replace github.com/pkg/sftp => ./third_party/sftp

replace golang.org/x/crypto => ./third_party/xcrypto
