package zmodem

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

type SendFile struct {
	Name   string
	Size   int64
	Reader io.ReaderAt
}
type FileProgress struct {
	Name         string
	Bytes, Total int64
	Index        int
}

// BatchResultError preserves skipped file names even if a later file fails.
// With no Cause, the batch handshake succeeded and the shell can be restored
// without transmitting another protocol abort.
type BatchResultError struct {
	Skipped []string
	Cause   error
}

func (e *BatchResultError) Error() string {
	message := "已跳过文件：" + strings.Join(e.Skipped, "、")
	if e.Cause != nil {
		message += "；" + e.Cause.Error()
	}
	return message
}
func (e *BatchResultError) Unwrap() error        { return e.Cause }
func (e *BatchResultError) Is(target error) bool { return e.Cause == nil && target == ErrSkipped }

func SendFiles(conn net.Conn, files []SendFile, progress func(FileProgress)) (result error) {
	var skipped []string
	defer func() {
		if len(skipped) > 0 {
			result = &BatchResultError{Skipped: skipped, Cause: result}
		}
	}()
	if len(files) == 0 {
		return fmt.Errorf("未选择上传文件")
	}
	s := newSender(conn, nil, "")
	defer s.reader.close()
	if _, e := conn.Write(BuildZhexHeader(FrameZRQINIT, 0)); e != nil {
		return e
	}
	if e := s.waitForZrinit(); e != nil {
		return e
	}
	for i, file := range files {
		if file.Size < 0 || file.Size > int64(^uint32(0)) || file.Name == "" || strings.ContainsAny(file.Name, "\x00\r\n/\\") {
			return fmt.Errorf("ZMODEM 文件名或大小无效")
		}
		s.filename = file.Name
		s.source = file.Reader
		s.size = file.Size
		s.onProgress = func(n, total int64) {
			if progress != nil {
				progress(FileProgress{file.Name, n, total, i})
			}
		}
		if e := s.sendCurrentFile(false); e != nil {
			if !errors.Is(e, ErrSkipped) {
				return fmt.Errorf("%s: %w", file.Name, e)
			}
			skipped = append(skipped, file.Name)
		}
	}
	if _, e := conn.Write(BuildZhexHeader(FrameZFIN, 0)); e != nil {
		return e
	}
	if e := s.waitForZfin(); e != nil {
		return e
	}
	_, e := conn.Write([]byte("OO"))
	return e
}

type ReceiveOptions struct{ ConfirmPrompt func([]byte) bool }
type ReceiveFile struct {
	Name string
	Size int64
}

// ReceiveFiles streams each file into the writer returned by open. A rejected
// file sends ZSKIP; no server-provided path is interpreted by this package.
func ReceiveFiles(conn net.Conn, open func(ReceiveFile) (io.WriteCloser, error), progress func(FileProgress), options ...ReceiveOptions) error {
	r := &receiver{conn: conn, reader: newReader(conn)}
	defer r.reader.close()
	if e := r.sendZRINIT(); e != nil {
		return e
	}
	index := 0
	for {
		frame, _, e := r.readHeader()
		if e != nil {
			return fmt.Errorf("ZMODEM 等待发送端文件信息：%w", e)
		}
		switch frame {
		case FrameZRQINIT:
			if e = r.sendZRINIT(); e != nil {
				return e
			}
		case FrameZSINIT:
			if e = r.receiveSenderInit(); e != nil {
				return e
			}
		case FrameZFILE:
			r.filename = ""
			r.totalSize = 0
			r.received = 0
			r.out = nil
			if e = r.readZFileSubpacket(); e != nil {
				return e
			}
			writer, e := open(ReceiveFile{r.filename, int64(r.totalSize)})
			if e != nil {
				if _, e = conn.Write(r.buildHeader(FrameZSKIP, 0)); e != nil {
					return e
				}
				continue
			}
			r.writer = writer
			name, total, current := r.filename, int64(r.totalSize), index
			r.cfg.OnStatus = func(s ReceiveStatus) {
				if progress != nil {
					progress(FileProgress{name, int64(s.Bytes), total, current})
				}
			}
			if _, e = conn.Write(r.buildHeader(FrameZRPOS, 0)); e != nil {
				if abort, ok := writer.(interface{ Abort(error) }); ok {
					abort.Abort(e)
				}
				writer.Close()
				return e
			}
			e = r.receiveData()
			if e == nil && r.totalSize > 0 && r.received != r.totalSize {
				e = io.ErrUnexpectedEOF
			}
			if e != nil {
				if abort, ok := writer.(interface{ Abort(error) }); ok {
					abort.Abort(e)
				}
			}
			closeErr := writer.Close()
			r.writer = nil
			if e != nil {
				return fmt.Errorf("ZMODEM 接收文件数据（%d / %d 字节）：%w", r.received, r.totalSize, e)
			}
			if closeErr != nil {
				return fmt.Errorf("ZMODEM 保存下载文件：%w", closeErr)
			}
			index++
			if e = r.sendZRINIT(); e != nil {
				return e
			}
		case FrameZFIN:
			if _, e = conn.Write(BuildZhexHeader(FrameZFIN, 0)); e != nil {
				return e
			}
			var confirm func([]byte) bool
			if len(options) > 0 {
				confirm = options[0].ConfirmPrompt
			}
			if e := r.waitFinalOO(confirm); e != nil {
				return fmt.Errorf("ZMODEM 等待传输结束确认：%w", e)
			}
			return nil
		case FrameZABORT, FrameZFERR:
			return ErrCancelled
		default:
			if e = r.sendZRINIT(); e != nil {
				return e
			}
		}
	}
}
func Cancel(conn io.Writer) { conn.Write(cancelEcho) }

func (r *receiver) waitFinalOO(confirm func([]byte) bool) error {
	// All file CRCs, ZEOF offsets and writer.Close calls have succeeded, and
	// the sender has ended the batch with ZFIN. OO only releases the line;
	// losing it must not hold the shell until the data activity timeout.
	// Keep a bounded grace period, acknowledge repeated ZFINs, and return
	// any shell output to the terminal instead of consuming it as protocol.
	deadline := time.Now().Add(3 * time.Second)
	var pending []byte
	defer func() { r.reader.unread(pending) }()
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 || len(pending) >= 8192 {
			return nil
		}
		b, e := r.reader.readByte(remaining)
		if errors.Is(e, io.EOF) || errors.Is(e, errActivityTimeout) {
			return nil
		}
		if e != nil {
			return e
		}
		switch b {
		case 0x11, 0x13, 0x91, 0x93:
			continue
		case '\r', '\n', 0x8a:
			if len(pending) == 0 {
				continue // ZHEX trailer, including lrzsz's high-bit LF.
			}
		}
		pending = append(pending, b)
		if string(pending) == "OO" {
			pending = nil
			return nil
		}
		// ZFIN is a hex header even when file data uses binary CRC32.
		// Inspect only a complete suffix, validate its CRC, then consume it
		// exactly once. One or several padding '*' bytes are legal.
		start := len(pending) - 17
		if start >= 0 && pending[start] == ZPAD && pending[start+1] == ZDLE && pending[start+2] == ZHEX {
			frame, _, ok := DecodeHexHeader(pending, start+3)
			if ok && (frame == FrameZFIN || frame == FrameZABORT || frame == FrameZFERR) {
				for start > 0 && pending[start-1] == ZPAD {
					start--
				}
				pending = pending[:start]
				if frame != FrameZFIN {
					return ErrCancelled
				}
				if _, e = r.conn.Write(BuildZhexHeader(FrameZFIN, 0)); e != nil {
					return e
				}
			}
		}
		if confirm != nil && confirm(pending) {
			return nil
		}
	}
}
