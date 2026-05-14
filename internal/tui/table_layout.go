package tui

import "github.com/charmbracelet/bubbles/table"

// columnSpec defines a table column. Exactly one column per layout should set
// Flex to true; that column absorbs the remaining terminal width.
type columnSpec struct {
	Title    string
	Width    int  // fixed width; ignored when Flex is true
	Flex     bool // if true, this column expands to fill available space
	MinWidth int  // minimum width when Flex is true
}

// tableLayout tracks column definitions with one flexible column and
// recomputes widths when the terminal is resized.
type tableLayout struct {
	specs     []columnSpec
	flexIdx   int
	flexWidth int
}

// cellPadding is the horizontal padding added by the default bubbles/table
// Cell style: lipgloss.NewStyle().Padding(0, 1) = 1 left + 1 right.
const cellPadding = 2

func newTableLayout(specs []columnSpec) tableLayout {
	l := tableLayout{specs: specs, flexIdx: -1}
	for i, s := range specs {
		if s.Flex {
			l.flexIdx = i
			l.flexWidth = s.MinWidth
			break
		}
	}
	return l
}

// recompute updates the flexible column width for the given terminal width
// and returns the resolved column definitions.
func (l *tableLayout) recompute(termWidth int) []table.Column {
	fixedTotal := 0
	for i, s := range l.specs {
		if i != l.flexIdx {
			fixedTotal += s.Width
		}
	}

	fw := termWidth - fixedTotal - cellPadding*len(l.specs)
	if l.flexIdx >= 0 && fw < l.specs[l.flexIdx].MinWidth {
		fw = l.specs[l.flexIdx].MinWidth
	}
	l.flexWidth = fw

	return l.columns()
}

// columns returns the column definitions using the current flexWidth.
func (l *tableLayout) columns() []table.Column {
	cols := make([]table.Column, len(l.specs))
	for i, s := range l.specs {
		w := s.Width
		if i == l.flexIdx {
			w = l.flexWidth
		}
		cols[i] = table.Column{Title: s.Title, Width: w}
	}
	return cols
}

// FlexWidth returns the current width of the flexible column.
func (l tableLayout) FlexWidth() int {
	return l.flexWidth
}
