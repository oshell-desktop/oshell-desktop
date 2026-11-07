package core

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"
)

// Desktop uses gzip instead of Apple's LZFSE. The distinct magic prevents a
// native OShell reader from interpreting this platform-local stream as LZFSE.
const agentAuditMagic = "OSHELL-DESKTOP-AUDIT-1\n"
const agentExportMagic = "OSHELL-DESKTOP-AUDIT-EXPORT-1\n"
const agentAuditEventLimit = 1 << 20

var ErrAgentAuditUnsealed = errors.New("审计未封口或尾段缺失")

func readAgentFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		clear(b)
		return nil, fmt.Errorf("Agent 文件超过大小限制")
	}
	return b, err
}

type AgentAuditEvent struct {
	Kind      string            `json:"kind"`
	Time      time.Time         `json:"time"`
	Operation string            `json:"operation,omitempty"`
	Bytes     []byte            `json:"bytes,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}
type AgentAuditIndex struct {
	ID      string    `json:"id"`
	Session string    `json:"session"`
	Client  string    `json:"client"`
	Title   string    `json:"title"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended,omitempty"`
	Tasks   []string  `json:"tasks"`
	Sealed  bool      `json:"sealed"`
	Size    int64     `json:"-"`
	Problem string    `json:"-"`
	Preview string    `json:"-"`
}
type AgentAudit struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	key      []byte
	index    AgentAuditIndex
	sequence uint64
	previous [32]byte
	err      error
}

func agentAuditKey(root, id string) ([]byte, error) {
	master, err := loadLocalVaultKey(filepath.Join(root, "agent"))
	if err != nil {
		return nil, err
	}
	defer clear(master)
	key := make([]byte, 32)
	_, err = io.ReadFull(hkdf.New(sha256.New, master, []byte(id), []byte(agentAuditMagic)), key)
	return key, err
}
func NewAgentAudit(root string, index AgentAuditIndex, screen []byte, cols, rows int) (*AgentAudit, error) {
	index.ID, index.Started, index.Tasks = NewID(), time.Now().UTC(), []string{}
	key, err := agentAuditKey(root, index.ID)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "agent", "archives")
	if err = os.MkdirAll(dir, 0700); err != nil {
		clear(key)
		return nil, err
	}
	path := filepath.Join(dir, index.ID+".oshell-audit")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		clear(key)
		return nil, err
	}
	a := &AgentAudit{file: f, path: path, key: key, index: index}
	_, err = f.WriteString(agentAuditMagic + index.ID)
	if err == nil {
		err = a.write(AgentAuditEvent{Kind: "baseline", Bytes: screen, Fields: map[string]string{"cols": fmt.Sprint(cols), "rows": fmt.Sprint(rows), "title": index.Title, "session": index.Session, "client": index.Client}})
	}
	if err == nil {
		err = a.saveIndex()
	}
	if err != nil {
		f.Close()
		clear(key)
		return nil, err
	}
	return a, nil
}
func (a *AgentAudit) saveIndex() error {
	b, err := json.Marshal(a.index)
	if err != nil {
		return err
	}
	defer clear(b)
	c, err := seal(a.key, b, agentAuditMagic+a.index.ID+"index")
	if err != nil {
		return err
	}
	return AtomicWrite(a.path+".index", c)
}
func agentEventAAD(id string, seq uint64, prev [32]byte) string {
	return fmt.Sprintf("%s%s:%d:%x", agentAuditMagic, id, seq, prev)
}
func (a *AgentAudit) write(event AgentAuditEvent) error {
	if a.err != nil {
		return a.err
	}
	if a.file == nil {
		return fmt.Errorf("审计已经结束")
	}
	event.Time = time.Now().UTC()
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	defer clear(b)
	if len(b) > agentAuditEventLimit {
		return fmt.Errorf("单条审计事件超过 1 MiB")
	}
	var compressed bytes.Buffer
	z := gzip.NewWriter(&compressed)
	_, err = z.Write(b)
	if err == nil {
		err = z.Close()
	}
	if err != nil {
		return err
	}
	c, err := seal(a.key, compressed.Bytes(), agentEventAAD(a.index.ID, a.sequence, a.previous))
	clear(compressed.Bytes())
	if err != nil {
		return err
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(c)))
	if _, err = a.file.Write(size[:]); err == nil {
		_, err = a.file.Write(c)
	}
	if err == nil {
		err = a.file.Sync()
	}
	if err != nil {
		a.err = fmt.Errorf("审计持久化失败，自动输入已停止：%w", err)
		return a.err
	}
	a.previous = sha256.Sum256(c)
	a.sequence++
	return nil
}
func (a *AgentAudit) Append(event AgentAuditEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.write(event)
}
func (a *AgentAudit) Task(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.write(AgentAuditEvent{Kind: "task_start", Fields: map[string]string{"task": id}}); err != nil {
		return err
	}
	a.index.Tasks = append(a.index.Tasks, id)
	if err := a.saveIndex(); err != nil {
		a.err = err
		return err
	}
	return nil
}
func (a *AgentAudit) Err() error { a.mu.Lock(); defer a.mu.Unlock(); return a.err }
func (a *AgentAudit) ID() string { return a.index.ID }
func (a *AgentAudit) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.file == nil {
		return a.err
	}
	err := a.write(AgentAuditEvent{Kind: "seal"})
	err = errors.Join(err, a.file.Close())
	a.file = nil
	if err == nil {
		a.index.Sealed = true
		a.index.Ended = time.Now().UTC()
		err = a.saveIndex()
	}
	clear(a.key)
	a.err = err
	return err
}
func validAgentID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
func agentAuditPath(root, id string) (string, error) {
	if !validAgentID(id) {
		return "", fmt.Errorf("存档编号无效")
	}
	return filepath.Join(root, "agent", "archives", id+".oshell-audit"), nil
}

// Events are consumed sequentially; a caller must not retain unbounded output.
// An unsealed/truncated stream returns an error after its authenticated prefix.
func ReadAgentAudit(ctx context.Context, root, id string, visit func(AgentAuditEvent) error) error {
	key, err := agentAuditKey(root, id)
	if err != nil {
		return err
	}
	defer clear(key)
	path, err := agentAuditPath(root, id)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return readAgentAudit(ctx, f, id, key, visit)
}
func readAgentAudit(ctx context.Context, r io.Reader, id string, key []byte, visit func(AgentAuditEvent) error) error {
	header := make([]byte, len(agentAuditMagic)+36)
	if _, err := io.ReadFull(r, header); err != nil || string(header) != agentAuditMagic+id {
		return fmt.Errorf("审计格式或编号不匹配")
	}
	var sequence uint64
	var previous [32]byte
	sealed := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var size [4]byte
		_, err := io.ReadFull(r, size[:])
		if err == io.EOF && sealed {
			return nil
		}
		if err == io.EOF {
			return ErrAgentAuditUnsealed
		}
		if err != nil {
			return fmt.Errorf("审计尾段截断")
		}
		if sealed {
			return fmt.Errorf("审计封口后存在额外数据")
		}
		n := binary.BigEndian.Uint32(size[:])
		if n < 28 || n > agentAuditEventLimit+4096 {
			return fmt.Errorf("审计事件长度无效")
		}
		ciphertext := make([]byte, n)
		if _, err = io.ReadFull(r, ciphertext); err != nil {
			return fmt.Errorf("审计事件截断")
		}
		zipped, err := unseal(key, ciphertext, agentEventAAD(id, sequence, previous))
		if err != nil {
			return fmt.Errorf("审计认证失败或事件顺序损坏")
		}
		previous = sha256.Sum256(ciphertext)
		sequence++
		z, err := gzip.NewReader(bytes.NewReader(zipped))
		if err != nil {
			clear(zipped)
			return fmt.Errorf("审计压缩内容无效")
		}
		b, err := io.ReadAll(io.LimitReader(z, agentAuditEventLimit+1))
		z.Close()
		clear(zipped)
		if err != nil || len(b) > agentAuditEventLimit {
			clear(b)
			return fmt.Errorf("审计解压失败或超限")
		}
		var event AgentAuditEvent
		err = json.Unmarshal(b, &event)
		clear(b)
		if err != nil || sequence == 1 && event.Kind != "baseline" {
			return fmt.Errorf("审计事件格式无效")
		}
		sealed = event.Kind == "seal"
		if visit != nil {
			if err = visit(event); err != nil {
				return err
			}
		}
	}
}
func ListAgentAudits(root string) ([]AgentAuditIndex, int64, error) {
	entries, err := os.ReadDir(filepath.Join(root, "agent", "archives"))
	if os.IsNotExist(err) {
		return []AgentAuditIndex{}, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	out := []AgentAuditIndex{}
	var size int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".oshell-audit") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".oshell-audit")
		if !validAgentID(id) {
			continue
		}
		index := AgentAuditIndex{ID: id, Problem: "索引缺失或无法认证"}
		path, _ := agentAuditPath(root, id)
		if stat, err := entry.Info(); err == nil {
			index.Size = stat.Size()
			size += index.Size
		}
		key, e := agentAuditKey(root, id)
		if e == nil {
			b, e := readAgentFile(path+".index", 256<<10)
			if e == nil {
				var plain []byte
				plain, e = unseal(key, b, agentAuditMagic+id+"index")
				if e == nil {
					var decoded AgentAuditIndex
					if json.Unmarshal(plain, &decoded) == nil && decoded.ID == id {
						decoded.Size = index.Size
						index = decoded
					}
					clear(plain)
				}
			}
			clear(key)
		}
		out = append(out, index)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.After(out[j].Started) })
	return out, size, nil
}
func SearchAgentAudits(ctx context.Context, root, query, client, session, task string, from, to time.Time) ([]AgentAuditIndex, error) {
	list, _, err := ListAgentAudits(root)
	if err != nil {
		return nil, err
	}
	out := []AgentAuditIndex{}
	for _, index := range list {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if client != "" && index.Client != client || session != "" && index.Session != session || !from.IsZero() && index.Started.Before(from) || !to.IsZero() && index.Started.After(to) {
			continue
		}
		if task != "" {
			found := false
			for _, id := range index.Tasks {
				found = found || id == task
			}
			if !found {
				continue
			}
		}
		found := query == ""
		tail := ""
		if !found {
			err := ReadAgentAudit(ctx, root, index.ID, func(e AgentAuditEvent) error {
				fields, _ := json.Marshal(e.Fields)
				v := tail + string(e.Bytes) + string(fields)
				if pos := strings.Index(v, query); pos >= 0 {
					found = true
					if index.Preview == "" {
						index.Preview = strings.ToValidUTF8(v[max(0, pos-64):min(len(v), pos+len(query)+128)], "�")
					}
				}
				if len(v) > len(query) {
					tail = v[len(v)-len(query):]
				} else {
					tail = v
				}
				return nil
			})
			if err != nil && !errors.Is(err, ErrAgentAuditUnsealed) {
				index.Problem = err.Error()
				index.Sealed = false
				found = true
			}
		}
		if found {
			out = append(out, index)
			if len(out) == 100 {
				break
			}
		}
	}
	return out, nil
}
func ExportAgentAudit(ctx context.Context, root, id, password, destination string) error {
	if len([]rune(password)) < 12 {
		return fmt.Errorf("导出密码至少 12 个字符")
	}
	if err := ReadAgentAudit(ctx, root, id, nil); err != nil {
		return err
	}
	key, err := agentAuditKey(root, id)
	if err != nil {
		return err
	}
	defer clear(key)
	envelope, err := SealShared(key, []byte(password))
	if err != nil {
		return err
	}
	path, _ := agentAuditPath(root, id)
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	f, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(destination)
		}
	}()
	if _, err = f.WriteString(agentExportMagic); err != nil {
		return err
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(envelope)))
	if _, err = f.Write(size[:]); err != nil {
		return err
	}
	if _, err = f.Write(envelope); err != nil {
		return err
	}
	_, err = io.Copy(f, updateContextReader{Context: ctx, reader: src})
	if err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	err = f.Close()
	ok = err == nil
	return err
}
func DeleteAgentAudit(root, id string) error {
	// The coordinator additionally rejects live writers, including unsealed ones.
	path, err := agentAuditPath(root, id)
	if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil {
		return err
	}
	err = os.Remove(path + ".index")
	if os.IsNotExist(err) {
		err = nil
	}
	return err
}
func agentReceiptName(client, key string) string {
	h := sha256.Sum256([]byte(client + "\x00" + key))
	return "intent-" + hex.EncodeToString(h[:]) + ".bin"
}
