package zmodem

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"oshell-desktop/internal/zmodem/testutil"
	"path/filepath"
	"testing"
)

func TestBatchPeerRejectionsRemainDistinct(t *testing.T) {
	for _, atEOF := range []bool{false, true} {
		for _, tc := range []struct {
			frame byte
			want  error
		}{{FrameZSKIP, ErrSkipped}, {FrameZABORT, ErrCancelled}, {FrameZFERR, ErrRemoteFile}} {
			t.Run(fmt.Sprintf("%d/eof=%v", tc.frame, atEOF), func(t *testing.T) {
				wire := BuildZhexHeader(FrameZRINIT, 0)
				if atEOF {
					wire = append(wire, BuildZhexHeader(FrameZRPOS, 0)...)
				}
				wire = append(wire, BuildZhexHeader(tc.frame, 0)...)
				wire = append(wire, BuildZhexHeader(FrameZFIN, 0)...)
				conn := &wireFixture{Reader: bytes.NewReader(wire)}
				err := SendFiles(conn, []SendFile{{Name: "first.bin", Reader: bytes.NewReader(nil)}}, nil)
				if !errors.Is(err, tc.want) {
					t.Fatalf("want %v, got %v", tc.want, err)
				}
				if tc.frame == FrameZSKIP {
					var result *BatchResultError
					if !errors.As(err, &result) || len(result.Skipped) != 1 || result.Skipped[0] != "first.bin" {
						t.Fatal("lost skipped file", err)
					}
				}
			})
		}
	}
}

func TestBatchSkipContinuesButAbortStops(t *testing.T) {
	for _, tc := range []struct {
		frame byte
		want  error
		next  bool
	}{{FrameZSKIP, ErrSkipped, true}, {FrameZABORT, ErrCancelled, false}, {FrameZFERR, ErrRemoteFile, false}} {
		t.Run(fmt.Sprint(tc.frame), func(t *testing.T) {
			wire := append(BuildZhexHeader(FrameZRINIT, 0), BuildZhexHeader(tc.frame, 0)...)
			wire = append(wire, BuildZhexHeader(FrameZRPOS, 0)...)
			wire = append(wire, BuildZhexHeader(FrameZRINIT, 0)...)
			wire = append(wire, BuildZhexHeader(FrameZFIN, 0)...)
			conn := &wireFixture{Reader: bytes.NewReader(wire)}
			err := SendFiles(conn, []SendFile{{Name: "first.bin", Reader: bytes.NewReader(nil)}, {Name: "second.bin", Reader: bytes.NewReader(nil)}}, nil)
			if !errors.Is(err, tc.want) {
				t.Fatalf("%v", err)
			}
			if bytes.Contains(conn.written.Bytes(), []byte("second.bin")) != tc.next {
				t.Fatal("incorrect continuation after peer rejection")
			}
		})
	}
}

func TestBatchRealRzProtectExistingReportsSkip(t *testing.T) {
	dir := t.TempDir()
	name := "synthetic.bin"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	conn, wait := testutil.LrzszPeer(t, dir, "rz", "-b", "-p")
	err := SendFiles(conn, []SendFile{{Name: name, Size: 3, Reader: bytes.NewReader([]byte("new"))}}, nil)
	wait()
	got, readErr := os.ReadFile(filepath.Join(dir, name))
	if readErr != nil || string(got) != "original" {
		t.Fatalf("existing file changed: %q %v", got, readErr)
	}
	if !errors.Is(err, ErrSkipped) {
		t.Fatalf("want skipped result, got %v", err)
	}
}
