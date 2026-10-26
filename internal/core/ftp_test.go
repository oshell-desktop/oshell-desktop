package core

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func ftpTestServer(t *testing.T, listingMode ...string) (Profile, string) {
	t.Helper()
	mode := ""
	if len(listingMode) > 0 {
		mode = listingMode[0]
	}
	root := t.TempDir()
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	var mu sync.Mutex
	var sockets []net.Conn
	var wg sync.WaitGroup
	t.Cleanup(func() {
		listener.Close()
		mu.Lock()
		for _, c := range sockets {
			c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, e := listener.Accept()
			if e != nil {
				return
			}
			mu.Lock()
			sockets = append(sockets, c)
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer c.Close()
				reader := bufio.NewReader(c)
				reply := func(code int, s string) { fmt.Fprintf(c, "%d %s\r\n", code, s) }
				reply(220, "test")
				cwd := "/"
				renameFrom := ""
				var data net.Listener
				defer func() {
					if data != nil {
						data.Close()
					}
				}()
				resolve := func(s string) string {
					if !strings.HasPrefix(s, "/") {
						s = path.Join(cwd, s)
					}
					return filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(path.Clean(s), "/")))
				}
				for {
					line, e := reader.ReadString('\n')
					if e != nil {
						return
					}
					command, arg, _ := strings.Cut(strings.TrimRight(line, "\r\n"), " ")
					switch command {
					case "USER":
						reply(331, "password")
					case "PASS":
						if arg == "test-only" {
							reply(230, "ok")
						} else {
							reply(530, "denied")
						}
					case "FEAT":
						fmt.Fprint(c, "211-Features\r\n MLST type*;size*;modify*;\r\n UTF8\r\n211 End\r\n")
					case "TYPE", "OPTS", "NOOP":
						reply(200, "ok")
					case "PWD":
						reply(257, "\""+cwd+"\"")
					case "CWD":
						if info, e := os.Stat(resolve(arg)); e == nil && info.IsDir() {
							if strings.HasPrefix(arg, "/") {
								cwd = path.Clean(arg)
							} else {
								cwd = path.Join(cwd, arg)
							}
							reply(250, "ok")
						} else {
							reply(550, "missing")
						}
					case "EPSV":
						if data != nil {
							data.Close()
						}
						data, e = net.Listen("tcp", "127.0.0.1:0")
						if e != nil {
							return
						}
						reply(229, fmt.Sprintf("Entering Extended Passive Mode (|||%d|)", data.Addr().(*net.TCPAddr).Port))
					case "MLSD", "LIST", "NLST", "STOR", "RETR":
						if (mode == "raw-list" && command == "MLSD") || (mode == "raw-nlst" && (command == "MLSD" || command == "LIST")) {
							reply(502, "listing unsupported")
							continue
						}
						if command == "MLSD" && (mode == "list" || mode == "nlst" || mode == "invalid-utf8") || command == "LIST" && (mode == "nlst" || mode == "invalid-utf8") {
							reply(502, "listing unsupported")
							continue
						}
						if data == nil {
							reply(425, "no data channel")
							continue
						}
						reply(150, "data")
						d, e := data.Accept()
						data.Close()
						data = nil
						if e != nil {
							return
						}
						success := true
						if command == "MLSD" || command == "LIST" || command == "NLST" {
							if strings.HasPrefix(mode, "raw-") && len(listingMode) > 1 {
								io.WriteString(d, listingMode[1])
								d.Close()
								reply(226, "complete")
								continue
							}
							entries, e := os.ReadDir(resolve(arg))
							if e != nil {
								success = false
							} else {
								for _, entry := range entries {
									if mode == "partial" && command == "MLSD" && entry.Name() == "data.txt" {
										continue
									}
									if command == "NLST" {
										fmt.Fprintf(d, "%s\r\n", entry.Name())
										continue
									}
									info, _ := entry.Info()
									kind := "file"
									if entry.IsDir() {
										kind = "dir"
									}
									fmt.Fprintf(d, "type=%s;size=%d;modify=20261009000000; %s\r\n", kind, info.Size(), entry.Name())
								}
								if mode == "partial" && command == "MLSD" {
									fmt.Fprint(d, "unrecognized listing row\r\n")
								}
								if mode == "invalid-utf8" && command == "NLST" {
									fmt.Fprint(d, "bad\xffname\r\n")
								}
							}
						} else if command == "STOR" {
							f, e := os.Create(resolve(arg))
							if e != nil {
								success = false
							} else {
								_, e = io.Copy(f, d)
								f.Close()
								success = e == nil
							}
						} else {
							f, e := os.Open(resolve(arg))
							if e != nil {
								success = false
							} else {
								_, e = io.Copy(d, f)
								f.Close()
								success = e == nil
							}
						}
						d.Close()
						if mode == "fail-retr" && command == "RETR" {
							success = false
						}
						if success {
							reply(226, "complete")
						} else {
							reply(550, "failed")
						}
					case "MKD":
						if e := os.Mkdir(resolve(arg), 0700); e == nil {
							reply(257, "created")
						} else {
							reply(550, "failed")
						}
					case "RNFR":
						renameFrom = resolve(arg)
						reply(350, "next")
					case "RNTO":
						if e := os.Rename(renameFrom, resolve(arg)); e == nil {
							reply(250, "ok")
						} else {
							reply(550, "failed")
						}
					case "DELE", "RMD":
						if e := os.Remove(resolve(arg)); e == nil {
							reply(250, "ok")
						} else {
							reply(550, "failed")
						}
					case "QUIT":
						reply(221, "bye")
						return
					default:
						reply(502, "unsupported")
					}
				}
			}()
		}
	}()
	p := testProfile(listener.Addr().String())
	p.Kind = "ftp"
	p.InitialDirectory = "/"
	return p, root
}
func TestFTPBrowserAndTransfers(t *testing.T) {
	p, remoteRoot := ftpTestServer(t)
	session, fs, e := LoginFTP(context.Background(), p, "test-only")
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	defer fs.Close()
	if _, e = fs.RealPath("/"); e != nil {
		t.Fatal(e)
	}
	local := t.TempDir()
	os.Mkdir(filepath.Join(local, "sub"), 0700)
	os.WriteFile(filepath.Join(local, "sub", "data.txt"), []byte("FTP roundtrip"), 0600)
	if e = fs.Upload(context.Background(), local, "/upload", nil); e != nil {
		t.Fatal(e)
	}
	if e = fs.Upload(context.Background(), local, "/upload", nil); e == nil {
		t.Fatal("overwrote existing directory")
	}
	target := filepath.Join(t.TempDir(), "download")
	if e = fs.Download(context.Background(), "/upload", target, false, nil); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(target, "sub", "data.txt"))
	if e != nil || string(b) != "FTP roundtrip" {
		t.Fatal(string(b), e)
	}
	if e = fs.Rename("/upload/sub/data.txt", "/upload/sub/renamed.txt"); e != nil {
		t.Fatal(e)
	}
	if e = fs.Remove("/upload/sub/renamed.txt"); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(remoteRoot, "upload", "sub", "renamed.txt")); !os.IsNotExist(e) {
		t.Fatal("remote deletion failed")
	}
	if _, _, e = LoginFTP(context.Background(), p, "bad"); e == nil {
		t.Fatal("bad password accepted")
	}
}
