package main

import (
	"fmt"
	"log"

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
		defer func() {
			showPanic(recover())
		}()

		checked := make(map[string]*tview.TableCell)

		flex := tview.NewFlex().SetDirection(tview.FlexRow)

		question := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText(question)
		flex.AddItem(question, 2, 3, true)

		button := tview.NewButton("Continue")
		button.SetSelectedFunc(func() {
			defer func() {
				showPanic(recover())
			}()

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

type LeafRet struct {
	leaves []*Leaf
}

func (lr *LeafRet) checkedNodesImpl(l *Leaf) {
	if l.checked {
		lr.leaves = append(lr.leaves, l)
	}

	for _, c := range l.leaves {
		lr.checkedNodesImpl(c)
	}
}

func checkedNodes(l *Leaf) []*Leaf {
	lr := LeafRet{}

	lr.leaves = make([]*Leaf, 0)

	lr.checkedNodesImpl(l)

	return lr.leaves
}

func (cw *ControlWindow) AskTree(questions []string, ls []*Leaf) []*Leaf {
	res := make(chan []*Leaf)

	go cw.app.QueueUpdateDraw(func() {
		defer func() {
			showPanic(recover())
		}()

		leavesFlex := tview.NewFlex()

		for i, l := range ls {
			currentLeaf := l

			leafFlex := tview.NewFlex().SetDirection(tview.FlexRow)

			questionIndex := 0
			if len(questions) > 1 {
				questionIndex = i
			}
			questionText := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText(questions[questionIndex])

			leafFlex.AddItem(questionText, 2, 3, true)

			button := tview.NewButton("Select")
			button.SetSelectedFunc(func() {
				defer func() {
					showPanic(recover())
				}()

				ls := checkedNodes(currentLeaf)
				leafFlex.Clear()
				cw.window.SetRoot(nil)

				res <- ls
			})

			buttonFlex := tview.NewFlex()
			buttonFlex.AddItem(nil, 0, 1, false)
			buttonFlex.AddItem(button, 10, 0, true)
			buttonFlex.AddItem(nil, 0, 1, false)

			leafFlex.AddItem(buttonFlex, 1, 2, true)

			tree := tview.NewTreeView().SetRoot(l.node).SetCurrentNode(l.node)
			tree.SetChangedFunc(func(node *tview.TreeNode) {
			})

			leafFlex.AddItem(tree, 0, 7, true)

			leavesFlex.AddItem(leafFlex, 0, 1, true)
		}

		cw.window.SetRoot(leavesFlex)
	})

	return <-res
}

type Leaf struct {
	leaves  map[string]*Leaf
	parent  *Leaf
	Label   string
	checked bool
	node    *tview.TreeNode
}

func (parent *Leaf) Add(l *Leaf) {
	if _, ok := parent.leaves[l.Label]; ok {
		return
	}

	parent.leaves[l.Label] = l

	l.parent = parent

	parent.node.AddChild(l.node)
}

func (l *Leaf) Uncheck() {
	l.checked = false

	label := fmt.Sprintf("( ) %s", l.Label)

	l.node.SetText(label)

	for _, c := range l.leaves {
		c.Uncheck()
	}
}

func (l *Leaf) Check() {
	l.checked = true

	label := fmt.Sprintf("(X) %s", l.Label)

	l.node.SetText(label)

	if l.parent != nil {
		l.parent.Check()
	}
}

func newLeaf(name string) *Leaf {
	var l Leaf

	l.leaves = make(map[string]*Leaf, 0)
	l.Label = name

	l.node = tview.NewTreeNode(l.Label)
	l.Uncheck()

	l.node.SetSelectedFunc(func() {
		defer func() {
			showPanic(recover())
		}()

		if l.checked {
			l.Uncheck()
		} else {
			l.Check()
		}
	})

	return &l
}

// TODO: removeme
func (cw *ControlWindow) testAskTree() {
	cwd := newLeaf("Change to /tmp")

	currentDirSize := newLeaf("du -sch .")
	filesInDir := newLeaf("ls *")
	touchFile := newLeaf("touch file")
	filesInDir.Add(touchFile)

	cwd.Add(currentDirSize)
	cwd.Add(filesInDir)

	number0 := newLeaf("Zero")
	number1 := newLeaf("One")
	number2 := newLeaf("Two")

	number0.Add(number1)
	number1.Add(number2)

	ls := cw.AskTree([]string{"What to do?"}, []*Leaf{cwd, number0})

	for _, l := range ls {
		log.Printf("  %s", l.Label)
	}
}

func newControlWindow(app *tview.Application, wm *winman.Manager) *ControlWindow {
	var cw ControlWindow

	cw.app = app
	cw.wm = wm

	_, _, width, height := wm.GetRect()

	cw.window = wm.NewWindow(). // create new window and add it to the window manager
					SetDraggable(true). // make window draggable around the screen
					SetResizable(true). // make the window resizable
					SetTitle("Control") // set the window title

	cw.window.SetRect(0, height/2, int(float32(width)*0.8), height-(height/2))

	cw.window.Show()

	app.Draw()

	return &cw
}
