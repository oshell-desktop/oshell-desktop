package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type MCPClient struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
	Digest  string    `json:"digest"`
}
type MCPClients struct {
	mu        sync.Mutex
	directory string
	clients   []MCPClient
	key       []byte
}

func OpenMCPClients(root string) (*MCPClients, error) {
	s := &MCPClients{directory: filepath.Join(root, "agent"), clients: []MCPClient{}}
	data, err := ReadBounded(filepath.Join(s.directory, "clients.bin"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		s.key, err = loadLocalVaultKey(s.directory)
		if err != nil {
			return nil, fmt.Errorf("MCP 本机密钥缺失或损坏，已保留配对记录")
		}
		block, _ := aes.NewCipher(s.key)
		gcm, _ := cipher.NewGCM(block)
		if len(data) < gcm.NonceSize()+gcm.Overhead() {
			return nil, fmt.Errorf("MCP 配对记录损坏")
		}
		plain, e := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], []byte("OShell.Desktop.MCP.clients.v1"))
		if e != nil {
			return nil, fmt.Errorf("MCP 配对记录无法解密")
		}
		defer clear(plain)
		var doc struct {
			Version int         `json:"version"`
			Clients []MCPClient `json:"clients"`
		}
		if json.Unmarshal(plain, &doc) != nil || doc.Version != 1 || len(doc.Clients) > 32 {
			return nil, fmt.Errorf("MCP 配对记录格式无效")
		}
		seen := map[string]bool{}
		for _, c := range doc.Clients {
			digest, e := hex.DecodeString(c.Digest)
			if c.ID == "" || seen[c.ID] || e != nil || len(digest) != 32 {
				return nil, fmt.Errorf("MCP 配对记录无效")
			}
			seen[c.ID] = true
		}
		s.clients = doc.Clients
	} else {
		s.key, err = localVaultKey(s.directory)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}
func (s *MCPClients) List() []MCPClient {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]MCPClient{}, s.clients...)
	for i := range out {
		out[i].Digest = ""
	}
	return out
}
func (s *MCPClients) save(clients []MCPClient) error {
	plain, err := json.Marshal(struct {
		Version int         `json:"version"`
		Clients []MCPClient `json:"clients"`
	}{1, clients})
	if err != nil {
		return err
	}
	defer clear(plain)
	block, _ := aes.NewCipher(s.key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	data := gcm.Seal(nonce, nonce, plain, []byte("OShell.Desktop.MCP.clients.v1"))
	return AtomicWrite(filepath.Join(s.directory, "clients.bin"), data)
}
func (s *MCPClients) Add(name string) (MCPClient, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 80 || strings.ContainsAny(name, "\r\n\x00") {
		return MCPClient{}, "", fmt.Errorf("客户端名称须为 1–80 个字符")
	}
	if len(s.clients) >= 32 {
		return MCPClient{}, "", fmt.Errorf("最多配对 32 个客户端")
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return MCPClient{}, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	clear(tokenBytes)
	digest := sha256.Sum256([]byte(token))
	c := MCPClient{ID: NewID(), Name: name, Created: time.Now().UTC(), Digest: hex.EncodeToString(digest[:])}
	next := append(append([]MCPClient{}, s.clients...), c)
	if err := s.save(next); err != nil {
		return MCPClient{}, "", err
	}
	s.clients = next
	c.Digest = ""
	return c, token, nil
}
func (s *MCPClients) Revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := []MCPClient{}
	for _, c := range s.clients {
		if c.ID != id {
			next = append(next, c)
		}
	}
	if len(next) == len(s.clients) {
		return fmt.Errorf("客户端不存在")
	}
	if err := s.save(next); err != nil {
		return err
	}
	s.clients = next
	return nil
}
func (s *MCPClients) Authenticate(token string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	digest := sha256.Sum256([]byte(token))
	want := hex.EncodeToString(digest[:])
	for _, c := range s.clients {
		if subtle.ConstantTimeCompare([]byte(c.Digest), []byte(want)) == 1 {
			return c.ID, nil
		}
	}
	return "", fmt.Errorf("MCP 配对凭据无效或已撤销")
}
func MCPClientConfiguration(executable, root, token, kind string) string {
	quote := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	if kind == "Codex" {
		return "[mcp_servers.oshell]\ncommand = " + quote(executable) + "\nargs = [\"--mcp-stdio\"]\n[mcp_servers.oshell.env]\nOSHELL_AGENT_ROOT = " + quote(root) + "\nOSHELL_AGENT_TOKEN = " + quote(token) + "\n"
	}
	b, _ := json.MarshalIndent(map[string]any{"mcp": map[string]any{"oshell": map[string]any{"type": "local", "command": []string{executable, "--mcp-stdio"}, "environment": map[string]string{"OSHELL_AGENT_ROOT": root, "OSHELL_AGENT_TOKEN": token}, "enabled": true}}}, "", "  ")
	return string(b)
}
