package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"storage-optimizer/internal/models"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("215")).Padding(0, 1)
	infoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("85")).Bold(true)
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	helpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).PaddingTop(1)
)

func catColor(cat models.Category) lipgloss.Style {
	m := map[models.Category]string{
		models.CategoryCache:     "81",
		models.CategoryLog:       "207",
		models.CategoryInstaller: "214",
		models.CategoryLargeOld:  "209",
		models.CategoryDuplicate: "175",
	}
	if c, ok := m[cat]; ok {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(c))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
}

// Result is returned when the dashboard exits.
type Result struct {
	Approved bool
	Selected map[string]bool
	MoveFn   func(map[string]bool) MoveOutcome
}

// MoveOutcome reports the quarantine operations result.
type MoveOutcome struct {
	Succeeded int
	Failed    int
	Freed     int64
	Err       error
}

// proceedMsg carries the async result of the quarantine move.
type proceedMsg struct {
	outcome MoveOutcome
}

// model is the Bubble Tea dashboard model.
type model struct {
	report   *models.ScanReport
	summary  []*models.CategorySummary
	selected map[string]bool
	cursor   int
	expanded int
	ready    bool
	viewport viewport.Model
	width    int
	height   int
	errMsg   string
	approved bool
	busy     bool
	outcome  MoveOutcome
	done     bool
	moveFn   func(map[string]bool) MoveOutcome
}

// New creates the dashboard model.
func New(report *models.ScanReport, moveFn func(map[string]bool) MoveOutcome) *model {
	sel := make(map[string]bool)
	summary := report.ByCategory()
	for _, cat := range summary {
		for _, f := range cat.Files {
			if f.Confidence >= 0.9 {
				sel[f.Key()] = true
			}
		}
	}
	return &model{
		report:   report,
		summary:  summary,
		selected: sel,
		expanded: -1,
		moveFn:   moveFn,
	}
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-5)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 5
		}
		m.viewport.SetContent(m.renderBody())
		return m, nil
	case proceedMsg:
		m.outcome = msg.outcome
		m.busy = false
		m.done = true
		if m.outcome.Err != nil {
			m.errMsg = m.outcome.Err.Error()
		}
		return m, nil
	case tea.KeyMsg:
		if m.done {
			// only allow quitting after done
			switch msg.String() {
			case "q", "ctrl+c", "enter":
				return m, tea.Quit
			}
			return m, nil
		}
		if m.busy {
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < m.totalRows()-1 {
				m.cursor++
			}
		case " ":
			if row := m.currentRow(); row.key != "" {
				if m.selected[row.key] {
					delete(m.selected, row.key)
				} else {
					m.selected[row.key] = true
				}
			}
		case "enter":
			if row := m.currentRow(); row.isCategory && row.catIdx >= 0 {
				if m.expanded == row.catIdx {
					m.expanded = -1
				} else {
					m.expanded = row.catIdx
				}
			}
		case "a", "A":
			if m.currentRow().isCategory && m.expanded >= 0 {
				cat := m.summary[m.currentRow().catIdx]
				for _, f := range cat.Files {
					if m.currentRow().key != "" || m.currentRow().isCategory {
						m.selected[f.Key()] = true
					}
				}
			} else {
				for _, cat := range m.summary {
					for _, f := range cat.Files {
						m.selected[f.Key()] = true
					}
				}
			}
		case "n", "N":
			if m.currentRow().isCategory && m.expanded >= 0 {
				cat := m.summary[m.currentRow().catIdx]
				for _, f := range cat.Files {
					delete(m.selected, f.Key())
				}
			} else {
				m.selected = make(map[string]bool)
			}
		case "y":
			if m.selectedCount() > 0 {
				m.busy = true
				return m, m.runMove()
			}
		}
	}
	if !m.done && m.ready {
		m.viewport.SetContent(m.renderBody())
	}
	return m, nil
}

// runMove returns a command executing the quarantine asynchronously.
func (m *model) runMove() tea.Cmd {
	sel := make(map[string]bool, len(m.selected))
	for k, v := range m.selected {
		sel[k] = v
	}
	return func() tea.Msg {
		return proceedMsg{outcome: m.moveFn(sel)}
	}
}

// rowRef identifies one selectable row.
type rowRef struct {
	isCategory bool
	catIdx     int
	fileIdx    int
	key        string
}

func (m *model) totalRows() int {
	n := len(m.summary)
	if m.expanded >= 0 && m.expanded < len(m.summary) {
		n += len(m.summary[m.expanded].Files)
	}
	return n
}

func (m *model) currentRow() rowRef {
	catCount := len(m.summary)
	if m.cursor < catCount {
		return rowRef{isCategory: true, catIdx: m.cursor}
	}
	if m.expanded >= 0 {
		idx := m.cursor - catCount
		if idx >= 0 && idx < len(m.summary[m.expanded].Files) {
			return rowRef{catIdx: m.expanded, fileIdx: idx, key: m.summary[m.expanded].Files[idx].Key()}
		}
	}
	return rowRef{}
}

func (m *model) selectedCount() int {
	c, _ := m.report.TotalSelected(m.selected)
	return c
}

func (m *model) View() string {
	if m.done {
		return m.renderDone()
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(" STORAGE OPTIMIZER "))
	b.WriteString("\n")
	b.WriteString(infoStyle.Render(fmt.Sprintf(" Scan: %s | File: %d | Total dipindai: %s",
		m.report.GeneratedAt.Format("2006-01-02 15:04:05"),
		m.report.FilesScanned, models.HumanBytes(m.report.TotalBytes))))
	b.WriteString("\n\n")
	if m.ready {
		b.WriteString(m.viewport.View())
	} else {
		b.WriteString(m.renderBody())
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(" Navigasi: ↑↓/jk  [space] centang  [enter] expand  [a] pilih semua  [n] kosongkan  [y] APPROVE & HAPUS  [q] keluar"))
	b.WriteString("\n" + m.footer())
	return b.String()
}

func (m *model) footer() string {
	count, size := m.report.TotalSelected(m.selected)
	return infoStyle.Render(fmt.Sprintf(" Total dipilih: %s %d file — %s akan dibebaskan",
		okStyle.Render("✔"), count, okStyle.Render(models.HumanBytes(size))))
}

func (m *model) renderBody() string {
	var b strings.Builder
	catCount := len(m.summary)
	for ci, cat := range m.summary {
		selectedInCat := 0
		for _, f := range cat.Files {
			if m.selected[f.Key()] {
				selectedInCat++
			}
		}
		cursor := "  "
		if m.cursor == ci && !(m.expanded >= 0 && ci < m.expanded) {
			cursor = infoStyle.Render("› ") + ""
			cursor = "› "
		}
		box := "[ ]"
		switch {
		case len(cat.Files) > 0 && selectedInCat == len(cat.Files):
			box = okStyle.Render("[x]")
		case selectedInCat > 0:
			box = warnStyle.Render("[~]")
		}
		expander := ""
		if len(cat.Files) > 0 {
			if m.expanded == ci {
				expander = okStyle.Render(" ▼")
			} else {
				expander = dimStyle.Render(" ▶")
			}
		}
		line := fmt.Sprintf("%s%s %s %5d file  %8s  Confidence: %.0f%%%s",
			cursor, box,
			catColor(cat.Category).Render(fmt.Sprintf("%-24s", cat.Category)),
			cat.Count, models.HumanBytes(cat.TotalBytes), cat.Confidence*100, expander)
		b.WriteString(line + "\n")

		if m.expanded == ci {
			for fi, f := range cat.Files {
				fileCursor := "  "
				if m.cursor == catCount+fi {
					fileCursor = "› "
				}
				fbox := "[ ]"
				if m.selected[f.Key()] {
					fbox = okStyle.Render("[x]")
				}
				name := f.Meta.Name()
				if len(name) > 44 {
					name = name[:44] + "…"
				}
				b.WriteString(fmt.Sprintf("   %s%s %8s  %s\n", fileCursor, fbox, models.HumanBytes(f.Meta.Size), name))
			}
		}
	}
	return b.String()
}

func (m *model) renderDone() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" PEMBERSIHAN SELESAI "))
	b.WriteString("\n\n")
	if m.errMsg != "" {
		b.WriteString(errStyle.Render(" ✘ Terjadi kesalahan: "+m.errMsg) + "\n")
	}
	b.WriteString(okStyle.Render(fmt.Sprintf(" ✔ %d file berhasil dipindahkan ke Quarantine (%s)",
		m.outcome.Succeeded, models.HumanBytes(m.outcome.Freed))) + "\n")
	if m.outcome.Failed > 0 {
		b.WriteString(warnStyle.Render(fmt.Sprintf(" ✘ %d file gagal / di-skip (sedang digunakan / sudah berubah)",
			m.outcome.Failed)) + "\n")
	}
	b.WriteString(infoStyle.Render(" ℹ File dapat dipulihkan via: storage-optimizer restore <file-id>") + "\n")
	b.WriteString(infoStyle.Render(" ℹ Pembersihan permanen otomatis setelah masa retensi (lihat config/config.yaml)") + "\n")
	b.WriteString(helpStyle.Render(" Tekan q untuk menutup laporan."))
	return b.String()
}

// Run launches the dashboard in blocking mode and returns the Result.
func Run(report *models.ScanReport, moveFn func(map[string]bool) MoveOutcome) (*Result, error) {
	m := New(report, moveFn)
	p := tea.NewProgram(m, tea.WithAltScreen())
	pf, err := p.Run()
	if err != nil {
		return nil, err
	}
	fm := pf.(*model)
	return &Result{
		Approved: fm.done,
		Selected: fm.selected,
	}, nil
}