package main

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/epiclabs-io/winman"
	"github.com/gdamore/tcell/v2"

	"github.com/rivo/tview"
	"golang.org/x/crypto/ssh"
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

type SshWindow struct {
	app           *tview.Application
	WindowContent *tview.TextView
	Window        *winman.WindowBase
	ansi          io.Writer
	cancel        context.CancelFunc
	sshClient     *ssh.Client
	running       sync.Mutex
	cancelMutex   sync.Mutex
	ssh           *Ssh
}

func (sw *SshWindow) Connect(addr string) {
	sw.sshClient = sw.ssh.Connect(addr)
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
		_, _, width, height = sw.WindowContent.GetInnerRect()
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

func newSshWindow(app *tview.Application, wm *winman.Manager, ssh *Ssh) *SshWindow {
	var sw SshWindow

	content := tview.NewTextView().SetDynamicColors(true).SetChangedFunc(func() { app.Draw() })
	content.SetScrollable(false)
	content.SetInputCapture(func(key *tcell.EventKey) *tcell.EventKey {
		return nil
	})

	sw.ansi = tview.ANSIWriter(content)
	sw.WindowContent = content
	sw.app = app

	sw.ssh = ssh

	return &sw
}

func newSshWindows(app *tview.Application, wm *winman.Manager, hosts []string) []*SshWindow {
	ssh := newSsh()

	sshWindows := make([]*SshWindow, len(hosts))

	for i, host := range hosts {
		sw := newSshWindow(app, wm, ssh)
		sshWindows[i] = sw

		sw.Connect(hosts[i])

		window := wm.NewWindow(). // create new window and add it to the window manager
						Show().                                // make window visible
						SetRoot(sw.WindowContent).             // have the text view above be the content of the window
						SetDraggable(true).                    // make window draggable around the screen
						SetResizable(true).                    // make the window resizable
						SetTitle(fmt.Sprintf("SSH %s", host)). // set the window title
						AddButton(&winman.Button{              // create a button with an X to close the application
				Symbol:  'X',
				OnClick: func() { app.Stop() }, // close the application
			})

		sw.Window = window

		window.SetRect(35*i, 5, 80, 50) // place the window
	}

	return sshWindows
}

func win() {
	app := tview.NewApplication()
	wm := winman.NewWindowManager()

	sshs := newSshWindows(app, wm, []string{"pi3", "pi4", "pi3/pi4"})

	go func() {
		sshs[0].Run("hostname")
		sshs[1].Run("hostname")
		sshs[2].Run("hostname")
		time.Sleep(time.Second * 3)
		go sshs[0].Run("find /")
		go sshs[1].Run("find / -type d")
		go sshs[2].Run("find / -type f")
		time.Sleep(time.Second * 2)
		sshs[0].Cancel()
		sshs[1].Cancel()
		sshs[2].Cancel()

		for _, ssh := range sshs {
			fmt.Fprintf(ssh.ansi, "\n------------------------\n")
		}
	}()

	// now, execute the application:
	if err := app.SetRoot(wm, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

func main() {
	win()
}
