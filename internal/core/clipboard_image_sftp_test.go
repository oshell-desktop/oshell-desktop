package core

import (
	"io"
	"net"
	"os"
	"path"
	"sync"
	"testing"

	"github.com/pkg/sftp"
)

// The host OS cannot emulate POSIX chmod on Windows. This SFTP peer records
// permission changes independently of the host filesystem, including a server
// that acknowledges chmod but ignores it.
type imagePermissionPeer struct {
	sftp.Handlers
	mu     sync.Mutex
	modes  map[string]os.FileMode
	ignore bool
}

func (p *imagePermissionPeer) Filecmd(r *sftp.Request) error {
	if r.Method == "Setstat" && r.AttrFlags().Permissions {
		if _, err := p.Handlers.FileList.Filelist(sftp.NewRequest("Stat", r.Filepath)); err != nil {
			return err
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		if !p.ignore {
			p.modes[r.Filepath] = os.FileMode(r.Attributes().Mode).Perm()
		}
		return nil
	}
	return p.Handlers.FileCmd.Filecmd(r)
}

type imageModeInfo struct {
	os.FileInfo
	mode os.FileMode
}

func (i imageModeInfo) Mode() os.FileMode { return i.FileInfo.Mode().Type() | i.mode }

type imageModeList struct {
	sftp.ListerAt
	peer    *imagePermissionPeer
	file    string
	listing bool
}

func (l imageModeList) ListAt(dst []os.FileInfo, offset int64) (int, error) {
	n, err := l.ListerAt.ListAt(dst, offset)
	l.peer.mu.Lock()
	defer l.peer.mu.Unlock()
	for i := 0; i < n; i++ {
		key := l.file
		if l.listing {
			key = path.Join(key, dst[i].Name())
		}
		mode, ok := l.peer.modes[key]
		if !ok {
			mode = 0777
		}
		dst[i] = imageModeInfo{dst[i], mode}
	}
	return n, err
}
func (p *imagePermissionPeer) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	l, err := p.Handlers.FileList.Filelist(r)
	if err != nil {
		return nil, err
	}
	return imageModeList{l, p, r.Filepath, r.Method == "List"}, nil
}
func imageSFTPClient(t *testing.T, ignorePermissions bool) *sftp.Client {
	t.Helper()
	p := &imagePermissionPeer{Handlers: sftp.InMemHandler(), modes: map[string]os.FileMode{}, ignore: ignorePermissions}
	h := p.Handlers
	h.FileCmd = p
	h.FileList = p
	left, right := net.Pipe()
	server := sftp.NewRequestServer(left, h)
	done := make(chan error, 1)
	go func() { done <- server.Serve() }()
	c, err := sftp.NewClientPipe(right, right)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c.Close()
		right.Close()
		server.Close()
		err := <-done
		if err != nil && err != io.EOF {
			t.Log(err)
		}
	})
	return c
}
