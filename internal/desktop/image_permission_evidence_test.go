package desktop

import (
	"os"
	"path"
	"sync"

	"github.com/pkg/sftp"
)

// The Windows host cannot implement remote POSIX mode bits. Keep those in the
// synthetic SFTP peer, as core's imagePermissionPeer does, while retaining the
// actual SSH/SFTP packets and production upload/reminder path.
type evidenceImagePeer struct {
	sftp.Handlers
	mu    sync.Mutex
	modes map[string]os.FileMode
}

func evidenceImagePermissions(h sftp.Handlers) sftp.Handlers {
	p := &evidenceImagePeer{Handlers: h, modes: map[string]os.FileMode{}}
	h.FileCmd = p
	h.FileList = p
	return h
}
func (p *evidenceImagePeer) Filecmd(r *sftp.Request) error {
	if r.Method == "Setstat" && r.AttrFlags().Permissions {
		if _, err := p.Handlers.FileList.Filelist(sftp.NewRequest("Stat", r.Filepath)); err != nil {
			return err
		}
		p.mu.Lock()
		p.modes[r.Filepath] = os.FileMode(r.Attributes().Mode).Perm()
		p.mu.Unlock()
		return nil
	}
	return p.Handlers.FileCmd.Filecmd(r)
}

type evidenceImageMode struct {
	os.FileInfo
	mode os.FileMode
}

func (i evidenceImageMode) Mode() os.FileMode { return i.FileInfo.Mode().Type() | i.mode }

type evidenceImageList struct {
	sftp.ListerAt
	p       *evidenceImagePeer
	file    string
	listing bool
}

func (l evidenceImageList) ListAt(dst []os.FileInfo, offset int64) (int, error) {
	n, err := l.ListerAt.ListAt(dst, offset)
	l.p.mu.Lock()
	defer l.p.mu.Unlock()
	for i := 0; i < n; i++ {
		name := l.file
		if l.listing {
			name = path.Join(name, dst[i].Name())
		}
		mode, ok := l.p.modes[name]
		if !ok {
			mode = 0777
		}
		dst[i] = evidenceImageMode{dst[i], mode}
	}
	return n, err
}
func (p *evidenceImagePeer) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	list, err := p.Handlers.FileList.Filelist(r)
	if err != nil {
		return nil, err
	}
	return evidenceImageList{list, p, r.Filepath, r.Method == "List"}, nil
}
