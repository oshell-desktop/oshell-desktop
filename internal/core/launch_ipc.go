package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

var ErrLaunchBusy = errors.New("此配置已有启动服务")

type launchRecent struct {
	done     chan struct{}
	response LaunchResponse
}
type LaunchServer struct {
	listener net.Listener
	release  func()
	handler  func(context.Context, ExternalLaunchRequest) error
	ctx      context.Context
	cancel   context.CancelFunc
	slots    chan struct{}
	mu       sync.Mutex
	recent   map[string]*launchRecent
	order    []string
	once     sync.Once
}

func StartLaunchServer(configuration string, handler func(context.Context, ExternalLaunchRequest) error) (*LaunchServer, error) {
	listener, release, err := listenLaunch(configuration)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := &LaunchServer{listener: listener, release: release, handler: handler, ctx: ctx, cancel: cancel, slots: make(chan struct{}, 8), recent: map[string]*launchRecent{}}
	go server.accept()
	return server, nil
}
func (s *LaunchServer) Close() error {
	var e error
	s.once.Do(func() {
		s.cancel()
		e = s.listener.Close()
		if s.release != nil {
			s.release()
		}
	})
	return e
}
func (s *LaunchServer) accept() {
	for {
		conn, e := s.listener.Accept()
		if e != nil {
			return
		}
		select {
		case s.slots <- struct{}{}:
			go s.serve(conn)
		default:
			conn.Close()
		}
	}
}
func (s *LaunchServer) serve(conn net.Conn) {
	defer conn.Close()
	defer func() { <-s.slots }()
	stop := context.AfterFunc(s.ctx, func() { conn.Close() })
	defer stop()
	if verifyLaunchConnection(conn, false) != nil {
		return
	}
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	var request ExternalLaunchRequest
	if readLaunchMessage(conn, &request) != nil || request.Validate() != nil {
		_ = writeLaunchMessage(conn, LaunchResponse{Message: "启动参数无效或通道读取失败"})
		return
	}
	conn.SetDeadline(time.Now().Add(300 * time.Second))
	key := request.ID()
	s.mu.Lock()
	entry, duplicate := s.recent[key]
	if !duplicate {
		entry = &launchRecent{done: make(chan struct{})}
		s.recent[key] = entry
	}
	s.mu.Unlock()
	if duplicate {
		select {
		case <-entry.done:
		case <-s.ctx.Done():
			return
		}
	} else {
		ctx, cancel := context.WithTimeout(s.ctx, 300*time.Second)
		e := s.handler(ctx, request)
		cancel()
		entry.response = LaunchResponse{Accepted: e == nil}
		if e != nil {
			entry.response.Message = "OShell 尚未解锁、正在关闭或启动请求未被接受"
		}
		s.mu.Lock()
		if e != nil {
			delete(s.recent, key)
		} else {
			s.order = append(s.order, key)
			if len(s.order) > 256 {
				delete(s.recent, s.order[0])
				s.order = s.order[1:]
			}
		}
		close(entry.done)
		s.mu.Unlock()
	}
	_ = writeLaunchMessage(conn, entry.response)
}
func SendExternalLaunch(ctx context.Context, configuration string, request ExternalLaunchRequest) error {
	if e := request.Validate(); e != nil {
		return e
	}
	conn, e := dialLaunch(ctx, configuration)
	if e != nil {
		return fmt.Errorf("OShell 启动服务尚未就绪")
	}
	defer conn.Close()
	if e = verifyLaunchConnection(conn, true); e != nil {
		return e
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline := time.Now().Add(300 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	if e = writeLaunchMessage(conn, request); e != nil {
		return e
	}
	var response LaunchResponse
	if e = readLaunchMessage(conn, &response); e != nil {
		return e
	}
	if !response.Accepted {
		return fmt.Errorf("%s", response.Message)
	}
	return nil
}
