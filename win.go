package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/epiclabs-io/winman"
	"github.com/gdamore/tcell/v2"

	"github.com/rivo/tview"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type readerCtx struct {
	ctx context.Context
	r   io.Reader
}

func (r *readerCtx) Read(p []byte) (n int, err error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

// NewReader gets a context-aware io.Reader.
func newReader(ctx context.Context, r io.Reader) io.Reader {
	return &readerCtx{ctx: ctx, r: r}
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

type SshWindow struct {
	app         *tview.Application
	Window      *tview.TextView
	ansi        io.Writer
	cancel      context.CancelFunc
	sshClient   *ssh.Client
	running     sync.Mutex
	cancelMutex sync.Mutex
}

func (sw *SshWindow) Connect(host string) {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	knownHostsPath := filepath.Join(usr.HomeDir, ".ssh", "known_hosts")

	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		panic(err)
	}

	auths := sshKeys()
	if len(auths) == 0 {
		panic("no auth method available")
	}

	config := &ssh.ClientConfig{
		User:            usr.Username,
		Auth:            auths,
		HostKeyCallback: hostKeyCallback,
		//HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	sw.sshClient, err = ssh.Dial("tcp", host+":22", config)
	if err != nil {
		panic(err)
	}

}

func (sw *SshWindow) Cancel() {
	sw.cancelMutex.Lock()
	defer sw.cancelMutex.Unlock()

	if sw.cancel != nil {
		sw.cancel()
		sw.running.Lock()
		defer sw.running.Unlock()
	}

	sw.cancel = nil

}

func (sw *SshWindow) Close() {
	sw.Cancel()
	sw.sshClient.Close()
}

func (sw *SshWindow) windowSize() (int, int) {
	var width int
	var height int

	sw.app.QueueUpdate(func() {
		_, _, width, height = sw.Window.GetInnerRect()
	})

	return width, height
}

func (sw *SshWindow) Run(cmd string) {
	sw.running.Lock()
	defer sw.running.Unlock()

	ss, err := sw.sshClient.NewSession()
	if err != nil {
		panic(err)
	}

	defer ss.Close()

	stdout, err := ss.StdoutPipe()
	if err != nil {
		panic(err)
	}
	stderr, err := ss.StderrPipe()
	if err != nil {
		panic(err)
	}
	combinedOutput := io.MultiReader(stdout, stderr)

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // supress echo
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	width, height := sw.windowSize()
	// run terminal session
	if err = ss.RequestPty("xterm", height, width, modes); err != nil {
		panic(err)
	}
	ss.Start(cmd)

	ctx, cancel := context.WithCancel(context.Background())
	r := newReader(ctx, combinedOutput)
	sw.cancelMutex.Lock()
	sw.cancel = cancel
	sw.cancelMutex.Unlock()

	for {
		n, err := io.Copy(sw.ansi, r)
		if err != nil || n == 0 {
			break
		}
	}
}

func newSshWindow(app *tview.Application, wm *winman.Manager) *SshWindow {
	var sw SshWindow

	content := tview.NewTextView().SetDynamicColors(true).SetChangedFunc(func() { app.Draw() })
	content.SetScrollable(false)
	content.SetInputCapture(func(key *tcell.EventKey) *tcell.EventKey {
		return nil
	})

	sw.ansi = tview.ANSIWriter(content)
	sw.Window = content
	sw.app = app

	return &sw
}

func win() {

	app := tview.NewApplication()
	wm := winman.NewWindowManager()

	sw := newSshWindow(app, wm)

	window := wm.NewWindow(). // create new window and add it to the window manager
					Show().                   // make window visible
					SetRoot(sw.Window).       // have the text view above be the content of the window
					SetDraggable(true).       // make window draggable around the screen
					SetResizable(true).       // make the window resizable
					SetTitle("SSH").          // set the window title
					AddButton(&winman.Button{ // create a button with an X to close the application
			Symbol:  'X',
			OnClick: func() { app.Stop() }, // close the application
		})

	window.SetRect(5, 5, 80, 50) // place the window

	sw.Connect("pi4")

	go func() {
		sw.Run("/home/btwotch/prog.sh")
		sw.Cancel()
		go func() {
			time.Sleep(time.Second * 8)
			sw.Cancel()
			for i := 0; i < 20; i++ {
				fmt.Fprintf(sw.ansi, "-------------------------------\n")
			}
			sw.Run("ls --color=always /")
		}()
		sw.Run("xxd /dev/urandom")
	}()
	// now, execute the application:
	if err := app.SetRoot(wm, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

func main() {
	win()
}
