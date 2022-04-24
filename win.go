package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
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
	app             *tview.Application
	WindowContent   *tview.TextView
	Window          *winman.WindowBase
	ansi            io.Writer
	logOutput       *os.File
	logOutputWriter io.Writer
	cancel          context.CancelFunc
	sshClient       *ssh.Client
	running         sync.Mutex
	cancelMutex     sync.Mutex
	ssh             *Ssh
}

func (sw *SshWindow) Connect(addr string) bool {
	sw.sshClient = sw.ssh.Connect(addr)

	if sw.sshClient == nil {
		return false
	}

	return true
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

func newSshWindow(app *tview.Application, wm *winman.Manager, addr, title string, ssh *Ssh) *SshWindow {
	var err error
	var sw SshWindow

	content := tview.NewTextView().SetDynamicColors(true).SetChangedFunc(func() { app.Draw() })
	content.SetScrollable(false)
	content.SetInputCapture(func(key *tcell.EventKey) *tcell.EventKey {
		return nil
	})

	sw.logOutput, err = os.Create(strings.ReplaceAll(title, "/", "_") + ".log")
	if err != nil {
		panic(err)
	}

	sw.logOutputWriter = bufio.NewWriter(sw.logOutput)
	ansi := tview.ANSIWriter(content)
	sw.ansi = io.MultiWriter(ansi, sw.logOutputWriter)
	sw.WindowContent = content
	sw.app = app

	sw.ssh = ssh

	window := wm.NewWindow(). // create new window and add it to the window manager
					SetRoot(sw.WindowContent). // have the text view above be the content of the window
					SetDraggable(true).        // make window draggable around the screen
					SetResizable(true).        // make the window resizable
					SetTitle(title)            // set the window title

	sw.Window = window

	return &sw
}

func organizeWindows(windows []*winman.WindowBase, x, y, width, height int) {
	windowWidth := width / len(windows)

	for i, win := range windows {
		windowX := x + i*windowWidth
		win.SetRect(windowX, y, windowWidth, y+height) // place the window
		win.Show()

	}
}

type SshWindows struct {
	windows map[string]*SshWindow
	ssh     *Ssh
	app     *tview.Application
	wm      *winman.Manager
}

func (sws *SshWindows) AddHost(addr, title string) *SshWindow {
	sw := newSshWindow(sws.app, sws.wm, addr, title, sws.ssh)

	if !sw.Connect(addr) {
		return nil
	}

	sws.windows[title] = sw
	sws.Reorganize()

	return sw
}

func (sws *SshWindows) Reorganize() {
	x, y, width, height := sws.wm.GetRect()
	height = height / 2

	windows := make([]*winman.WindowBase, len(sws.windows))

	i := 0
	for _, win := range sws.windows {
		windows[i] = win.Window
		i++
	}

	organizeWindows(windows, x, y, width, height)
	sws.app.Draw()
}

func (sws *SshWindows) RemoveHost(addr string) {
	sw, ok := sws.windows[addr]

	if !ok {
		return
	}

	sw.Cancel()
	sw.Window.Hide()

	delete(sws.windows, addr)

	sws.Reorganize()

	sw.logOutput.Close()
}

func newSshWindows(app *tview.Application, wm *winman.Manager, hosts []string) *SshWindows {
	var sws SshWindows

	sws.windows = make(map[string]*SshWindow)
	sws.ssh = newSsh()
	sws.app = app
	sws.wm = wm

	windows := make([]*winman.WindowBase, 0)

	x, y, width, height := wm.GetRect()
	height = height / 2
	for _, host := range hosts {
		sw := sws.AddHost(host, fmt.Sprintf("SSH %s", host))
		if sw == nil {
			continue
		}

		windows = append(windows, sw.Window)
		organizeWindows(windows, x, y, width, height)
		app.Draw()
	}

	return &sws
}

func (sws *SshWindows) Host(addr string) *SshWindow {
	return sws.windows[addr]
}

func (sws *SshWindows) Hosts() []*SshWindow {
	sshWindows := make([]*SshWindow, len(sws.windows))
	i := 0
	for _, window := range sws.windows {
		sshWindows[i] = window
		i++
	}
	return sshWindows
}

func win() {
	defer func() {
		showPanic(recover())
	}()

	app := tview.NewApplication()
	wm := winman.NewWindowManager()

	go func() {
		app.QueueUpdate(func() {
			go func() {
				defer func() {
					showPanic(recover())
				}()

				setupLogWindow(app, wm)
				fmt.Fprintf(LW.Ansi, "Started\n")
				sws := newSshWindows(app, wm, []string{})
				ctrl := newControlWindow(app, wm)
				pis := make([]string, 100)
				for i, _ := range pis {
					pis[i] = fmt.Sprintf("pi%d", i)
				}
				res := ctrl.Ask("Which PI?", pis, false)
				//res := ctrl.Ask2("Which PI?", []string{"pi3", "pi4", "pi3/pi4", "asdf"})
				for _, host := range res {
					sws.AddHost(host, host)
				}
				time.Sleep(time.Second * 3)

				for _, ssh := range sws.Hosts() {
					ssh.Cancel()
					fmt.Fprintf(ssh.ansi, "\n------------------------\n")
					ssh.Run("hostname")
					ssh.Run("df -h")
				}
			}()
		})
	}()

	// now, execute the application:
	if err := app.SetRoot(wm, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

func main() {
	win()
}
