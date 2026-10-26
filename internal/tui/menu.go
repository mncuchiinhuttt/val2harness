package tui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

type KeyAction int

const (
	ActionNone KeyAction = iota
	ActionUp
	ActionDown
	ActionEnter
	ActionQuit
)

func readKey(fd int) (KeyAction, error) {
	var buf [3]byte
	n, err := os.Stdin.Read(buf[:1])
	if err != nil || n == 0 {
		return ActionNone, err
	}

	b := buf[0]
	switch b {
	case 3, 'q', 'Q': // Ctrl+C or q
		return ActionQuit, nil
	case 13, 10: // Enter
		return ActionEnter, nil
	case 'k', 'K': // Vim up
		return ActionUp, nil
	case 'j', 'J': // Vim down
		return ActionDown, nil
	case 27: // Escape sequence
		n, err := os.Stdin.Read(buf[1:2])
		if err != nil || n == 0 {
			return ActionQuit, nil
		}
		if buf[1] == 91 { // '['
			n, err = os.Stdin.Read(buf[2:3])
			if err != nil || n == 0 {
				return ActionNone, nil
			}
			switch buf[2] {
			case 65: // Up arrow
				return ActionUp, nil
			case 66: // Down arrow
				return ActionDown, nil
			}
		}
	}
	return ActionNone, nil
}

func SelectMenu(statusHeader string, items []string, selectedIdx int) int {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return -1
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return -1
	}
	defer func() {
		_ = term.Restore(fd, oldState)
		fmt.Print("\x1b[?25h") // restore cursor
	}()

	fmt.Print("\x1b[?25l") // hide cursor

	for {
		// Clear screen and draw menu (Claude Code style)
		fmt.Print("\x1b[H\x1b[2J")
		fmt.Print(statusHeader)

		for i, item := range items {
			if i == selectedIdx {
				// Claude Code active indicator: bold cyan with ❯ pointer
				fmt.Printf("\x1b[1;36m❯ %s\x1b[0m\r\n", item)
			} else {
				// Inactive item: indented
				fmt.Printf("  %s\r\n", item)
			}
		}

		fmt.Print("\r\n\x1b[90mUse ↑/↓ to navigate · Enter to select · Esc/q to exit\x1b[0m\r\n")

		action, err := readKey(fd)
		if err != nil || action == ActionQuit {
			return -1
		}

		switch action {
		case ActionUp:
			selectedIdx--
			if selectedIdx < 0 {
				selectedIdx = len(items) - 1
			}
		case ActionDown:
			selectedIdx++
			if selectedIdx >= len(items) {
				selectedIdx = 0
			}
		case ActionEnter:
			return selectedIdx
		}
	}
}
