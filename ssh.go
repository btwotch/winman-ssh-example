package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Ssh struct {
	clients map[string]*ssh.Client
}

func newSsh() *Ssh {
	var s Ssh

	s.clients = make(map[string]*ssh.Client)

	return &s
}

func localHostKeys() ssh.HostKeyCallback {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	knownHostsPath := filepath.Join(usr.HomeDir, ".ssh", "known_hosts")

	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		panic(err)
	}

	return hostKeyCallback
}

func (s *Ssh) connectWithConn(conn net.Conn, addr string) *ssh.Client {
	fmt.Fprintf(os.Stderr, "Connecting to %s\n", addr)

	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	auths := sshKeys()
	if len(auths) == 0 {
		panic("no auth method available")
	}

	config := &ssh.ClientConfig{
		User: usr.Username,
		Auth: auths,
		//HostKeyCallback: localHostKeys(),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		panic(err)
	}

	return ssh.NewClient(c, chans, reqs)
}

func (s *Ssh) Connect(addressLine string) *ssh.Client {
	var err error
	var ok bool

	addrs := strings.Split(addressLine, "/")

	if len(addrs) == 0 {
		return nil
	}

	for i, addr := range addrs {
		if !strings.Contains(addr, ":") {
			addr = addr + ":22"
			addrs[i] = addr
		}
	}

	var c net.Conn
	var client *ssh.Client

	currAddrLine := addrs[0]
	client, ok = s.clients[currAddrLine]
	if !ok {
		c, err = net.Dial("tcp", addrs[0])
		if err != nil {
			panic(err)
		}

		client = s.connectWithConn(c, addrs[0])

		s.clients[currAddrLine] = client
	}

	for i := 1; i < len(addrs); i++ {
		prevAddrLine := currAddrLine

		addr := addrs[i]
		currAddrLine += "/" + addr

		client, ok = s.clients[currAddrLine]
		if ok {
			continue
		}

		conn, err := s.clients[prevAddrLine].Dial("tcp", addrs[i])
		if err != nil {
			panic(err)
		}
		client = s.connectWithConn(conn, addrs[i])

		s.clients[currAddrLine] = client
	}

	return client
}

func sshKeys() []ssh.AuthMethod {
	auths := make([]ssh.AuthMethod, 0)

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	sshDir := filepath.Join(usr.HomeDir, ".ssh")

	files, err := ioutil.ReadDir(sshDir)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), "id_") && !strings.HasSuffix(file.Name(), ".pub") {
			sshKeyFilePath := filepath.Join(sshDir, file.Name())
			key, err := ioutil.ReadFile(sshKeyFilePath)
			if err != nil {
				continue
			}

			signer, err := ssh.ParsePrivateKey(key)
			if err != nil {
				continue
			}
			auths = append(auths, ssh.PublicKeys(signer))
		}
	}

	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return auths
	}

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return auths
	}

	agentClient := agent.NewClient(conn)

	auths = append(auths, ssh.PublicKeysCallback(agentClient.Signers))

	return auths
}
