package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/epiclabs-io/winman"
	"github.com/rivo/tview"
)

type LogWindow struct {
	content *tview.TextView
	Ansi    io.Writer
	window  *winman.WindowBase
	wm      *winman.Manager
	app     *tview.Application
}

func showPanic(err interface{}) {
	if err == nil {
		return
	}

	errMsg := fmt.Sprintf("!!PANIC!!\n%s\n\n%s\n", err, string(debug.Stack()))

	if LW == nil {
		fmt.Fprintf(os.Stderr, errMsg)
	} else {
		LW.window.Maximize()
		LW.wm.SetZ(LW.window, winman.WindowZTop)
		LW.app.SetRoot(LW.window, true)

		LW.window.AddButton(&winman.Button{
			Symbol:  'X',
			OnClick: func() { LW.app.Stop() },
		})
		fmt.Fprintf(LW.Ansi, errMsg)
	}
}

func setupLogWindow(app *tview.Application, wm *winman.Manager) {
	var lw LogWindow

	lw.wm = wm
	lw.app = app
	lw.content = tview.NewTextView().SetTextAlign(tview.AlignLeft).SetChangedFunc(func() { app.Draw() })
	lw.window = wm.NewWindow(). // create new window and add it to the window manager
					SetRoot(lw.content). // have the text view above be the content of the window
					SetDraggable(true).  // make window draggable around the screen
					SetResizable(true).  // make the window resizable
					SetTitle("Log")      // set the window title

	lw.Ansi = tview.ANSIWriter(lw.content)

	_, _, width, height := wm.GetRect()
	lw.window.SetRect(int(float32(width)*0.8), height/2, width-int(float32(width)*0.8), height-(height/2))
	lw.window.Show()
	app.Draw()

	LW = &lw
}

var LW *LogWindow
