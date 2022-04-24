package main

import (
	"fmt"

	"github.com/epiclabs-io/winman"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type ControlWindow struct {
	window *winman.WindowBase
	app    *tview.Application
	wm     *winman.Manager
}

func checkCell(cell *tview.TableCell) {
	cell.SetTextColor(tcell.ColorBlack)
	cell.SetBackgroundColor(tcell.ColorWhite)
}

func uncheckCell(cell *tview.TableCell) {
	cell.SetTextColor(tcell.ColorWhite)
	cell.SetBackgroundColor(tcell.ColorBlack)
}

func (cw *ControlWindow) Ask(question string, options []string, onlyOne bool) []string {
	res := make(chan []string)

	go cw.app.QueueUpdateDraw(func() {
		checked := make(map[string]*tview.TableCell)

		flex := tview.NewFlex().SetDirection(tview.FlexRow)

		question := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText(question)
		flex.AddItem(question, 2, 3, true)

		button := tview.NewButton("Continue")
		button.SetSelectedFunc(func() {
			checkedArray := make([]string, 0)
			for opt, _ := range checked {
				checkedArray = append(checkedArray, opt)
			}

			flex.Clear()
			cw.window.SetRoot(nil)
			res <- checkedArray
		})

		buttonFlex := tview.NewFlex()
		buttonFlex.AddItem(nil, 0, 1, false)
		buttonFlex.AddItem(button, 10, 0, true)
		buttonFlex.AddItem(nil, 0, 1, false)

		flex.AddItem(buttonFlex, 1, 2, true)

		table := tview.NewTable().SetBorders(true)

		table.SetFixed(1, 0)
		table.SetSelectable(false, false)

		row, col := 0, 0
		longestOptionLen := 0
		for _, opt := range options {
			if len(opt) > longestOptionLen {
				longestOptionLen = len(opt)
			}
		}
		longestOptionLen += 1

		_, _, width, _ := cw.window.GetRect()
		width = width - 3
		maxCol := width / longestOptionLen

		for _, opt := range options {
			cell := tview.NewTableCell(opt)
			cell.SetAlign(tview.AlignCenter)
			uncheckCell(cell)

			option := opt
			cell.SetClickedFunc(func() bool {
				_, ok := checked[option]

				if onlyOne {
					for k, cell := range checked {
						uncheckCell(cell)
						delete(checked, k)
					}
				}

				if ok {
					uncheckCell(cell)
					delete(checked, option)
				} else {
					checkCell(cell)
					checked[option] = cell
				}

				return true
			})
			table.SetCell(row, col, cell)
			col++
			if col >= maxCol {
				row++
				col = 0
			}
		}

		flex.AddItem(table, 0, 7, true)

		cw.window.SetRoot(flex)
	})

	return <-res
}


func newControlWindow(app *tview.Application, wm *winman.Manager) *ControlWindow {
	var cw ControlWindow

	cw.app = app
	cw.wm = wm

	_, _, width, height := wm.GetRect()

	cw.window = wm.NewWindow(). // create new window and add it to the window manager
		//SetRoot(sw.WindowContent). // have the text view above be the content of the window
		SetDraggable(true). // make window draggable around the screen
		SetResizable(true). // make the window resizable
		SetTitle("Control") // set the window title

	cw.window.SetRect(0, height/2, int(float32(width)*0.8), height-(height/2))

	cw.window.Show()

	app.Draw()

	return &cw
}
