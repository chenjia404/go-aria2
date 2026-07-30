package sftpproto

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type testSFTPServer struct {
	t        *testing.T
	listener net.Listener
	handler  sftp.Handlers
	remote   string
	user     string
	password string
}

func newTestSFTPServer(t *testing.T, payload []byte) *testSFTPServer {
	t.Helper()

	signer := mustTestSigner(t)
	config := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if string(pass) == "secret" {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected")
		},
	}
	config.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := &testSFTPServer{
		t:        t,
		listener: ln,
		handler:  sftp.InMemHandler(),
		remote:   "/file.bin",
		user:     "root",
		password: "secret",
	}
	go srv.serve(config)

	if err := srv.seedFile(payload); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	return srv
}

func (s *testSFTPServer) addr() string {
	return s.listener.Addr().String()
}

func (s *testSFTPServer) close() {
	_ = s.listener.Close()
}

func (s *testSFTPServer) uri() string {
	host, port, err := net.SplitHostPort(s.addr())
	if err != nil {
		s.t.Fatalf("split addr: %v", err)
	}
	return fmt.Sprintf("sftp://%s:%s@%s:%s%s", s.user, s.password, host, port, s.remote)
}

func (s *testSFTPServer) seedFile(payload []byte) error {
	config := &ssh.ClientConfig{
		User:            s.user,
		Auth:            []ssh.AuthMethod{ssh.Password(s.password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // 测试专用
		Timeout:         0,
	}
	client, err := ssh.Dial("tcp", s.addr(), config)
	if err != nil {
		return err
	}
	defer client.Close()

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	file, err := sftpClient.Create(s.remote)
	if err != nil {
		return err
	}
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (s *testSFTPServer) serve(config *ssh.ServerConfig) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn, config)
	}
}

func (s *testSFTPServer) handleConn(conn net.Conn, config *ssh.ServerConfig) {
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go func(in <-chan *ssh.Request) {
			for req := range in {
				ok := false
				if req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp" {
					ok = true
				}
				_ = req.Reply(ok, nil)
			}
		}(requests)

		server := sftp.NewRequestServer(channel, s.handler)
		if err := server.Serve(); err != nil && err != io.EOF {
			_ = server.Close()
			continue
		}
		_ = server.Close()
	}
}

func mustTestSigner(t *testing.T) ssh.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	return signer
}
