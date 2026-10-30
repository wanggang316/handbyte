package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/wanggang316/handbyte/internal/cleaner"
	"github.com/wanggang316/handbyte/internal/scanner"
)

// File names and subprocess errors must not inject terminal control sequences.
func display(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
func (m *Model) line(s string) string { return ansi.Truncate(s, max(1, m.width), "…") }
func (m *Model) frame(lines []string) string {
	if len(lines) > m.height && m.height > 0 {
		lines = lines[:m.height]
	}
	for i := range lines {
		lines[i] = m.line(lines[i])
	}
	return strings.Join(lines, "\n")
}

func (m *Model) View() string {
	if m.width == 0 {
		return "Loading…"
	}
	if m.detailsOpen {
		return m.renderDetails()
	}
	if m.helpOpen {
		return m.renderHelp()
	}
	switch m.phase {
	case phaseSplash:
		return m.renderSplash()
	case phaseScanning, phaseList:
		return m.renderList()
	case phaseConfirm:
		return m.renderConfirm()
	case phaseCleaning:
		return m.renderCleaning()
	case phaseDone:
		return m.renderDone()
	}
	return ""
}

func (m *Model) renderSplash() string {
	roots := m.cfg.ProjectRoots
	if len(roots) == 0 {
		roots = scanner.DefaultProjectRoots
	}
	return m.frame([]string{
		titleStyle.Render("handbyte"), "", "Review the cost before you clean.", "",
		"Free space: " + humanBytes(m.freeBefore) + "    Action: " + m.actionLabel(),
		"Scope: " + m.scopeLabel(),
		"Project roots: " + display(strings.Join(roots, ", ")),
		fmt.Sprintf("Protected paths: %d", len(m.cfg.Exclusions)), "",
		"[1] Quick cleanup     Known caches and Xcode data",
		"[2] Project cleanup   Build output grouped by project",
		"[3] Explore storage   Browse files without cleanup actions", "",
		"[Enter] Scan current scope   [q] Quit   [?] Help",
		display(m.notice),
	})
}

func (m *Model) renderList() string {
	state := "Review"
	if m.phase == phaseScanning {
		state = m.spin.View() + " Scanning"
	}
	counts := map[scanner.Eligibility]int{}
	partial := 0
	for _, it := range m.items {
		counts[it.Eligibility]++
		if it.ScanStatus != "complete" {
			partial++
		}
	}
	lines := []string{
		titleStyle.Render("handbyte") + "  " + state + " · " + display(m.scopeLabel()),
		fmt.Sprintf("%d recommended · %d review · %d protected · %d incomplete", counts[scanner.Recommended], counts[scanner.Review], counts[scanner.Protected], partial),
		fmt.Sprintf("%d selected · %s estimated · %s", len(m.selectedItems()), humanBytes(m.selectedTotal()), m.actionLabel()),
		fmt.Sprintf("Filter: %s  Sort: %s  Search: %s", []string{"All", "Recommended", "Review", "Protected"}[m.filter], []string{"Size", "Name", "Activity age"}[m.sortMode], display(m.query)),
	}
	h := m.listHeight()
	start := max(0, m.cursor-h+1)
	end := min(len(m.rows), start+h)
	if len(m.rows) == 0 {
		text := "No matching items. Change the filter or scan scope."
		if m.phase == phaseScanning {
			text = "Discovering candidates…"
		}
		lines = append(lines, text)
		for i := 1; i < h; i++ {
			lines = append(lines, "")
		}
	} else {
		for i := start; i < end; i++ {
			lines = append(lines, m.renderRow(m.rows[i], i == m.cursor))
		}
		for i := end - start; i < h; i++ {
			lines = append(lines, "")
		}
	}
	details := m.detailLines()
	lines = append(lines, details...)
	notice := m.notice
	if m.phase == phaseScanning {
		notice = "Scanning: " + m.scanProg.CurrentPath + " · Esc stops the scan"
	}
	if m.editingQuery {
		notice = "Search: " + m.query + "▏  Enter keeps filter; Ctrl+U clears"
	}
	lines = append(lines, mutedStyle.Render(display(notice)),
		"[Space] Select [A] Recommended [d] Review [c] Clear [q] Quit",
		"[/] Search [f] Filter [s] Sort [p] Protect [e] Explore [o] Finder [?] Help")
	return m.frame(lines)
}

func (m *Model) renderRow(r row, current bool) string {
	var text string
	if r.isGroup {
		marker := "▾"
		if m.collapsed[r.group] {
			marker = "▸"
		}
		text = " " + marker + " " + groupStyle.Render(display(r.group))
	} else {
		it := r.item
		mark := "[ ]"
		if !it.Selectable() {
			mark = "[·]"
		} else if m.selected[it.Path] {
			mark = "[✓]"
		}
		name := it.Name
		if name == "" {
			name = it.Path
		}
		size := humanBytes(it.Size)
		if it.ScanStatus == "pending" {
			size = "pending"
		} else if it.ScanStatus == "unavailable" {
			size = "unknown"
		} else if it.ScanStatus == "partial" || it.ScanStatus == "cancelled" {
			size += "+"
		}
		text = fmt.Sprintf("   %s %10s  %s", mark, size, display(name))
	}
	text = m.line(text)
	if current {
		return rowCursorStyle.Render(text)
	}
	return text
}

func (m *Model) detailLines() []string {
	lines := []string{strings.Repeat("─", max(1, m.width)), "", "", "", ""}
	it := m.currentItem()
	if it == nil {
		lines[1] = "Select a row to see ownership, activity and cleanup cost."
		lines[2] = "Protected items can be explored or revealed in Finder."
		return lines
	}
	lines[1] = display(it.Name) + " · " + display(string(it.Eligibility)) + " · " + display(it.Reason)
	lines[2] = display(condensePath(it.Path))
	lines[3] = display(it.Description)
	activity := string(it.Activity)
	if activity == "" {
		activity = "not established"
	}
	age := "unknown"
	if !it.ModTime.IsZero() {
		age = fmt.Sprintf("%dd ago", int(time.Since(it.ModTime).Hours()/24))
		if time.Since(it.ModTime) < 24*time.Hour {
			age = "today"
		}
	}
	lines[4] = fmt.Sprintf("File activity: %s · Owner: %s · Scan: %s", age, activity, it.ScanStatus)
	if it.ScanError != "" {
		lines[4] += " · " + display(it.ScanError)
	}
	return lines
}

func (m *Model) renderConfirm() string {
	items := m.selectedItems()
	h := m.reviewHeight()
	start := min(m.reviewOffset, max(0, len(items)-h))
	end := min(len(items), start+h)
	consequence := "Items remain in Trash. This does not reclaim disk space."
	if m.cleanMode == cleaner.ModeRemove {
		consequence = "Permanent deletion cannot be undone. Rebuilds or downloads may be required."
	}
	if m.cleanMode == cleaner.ModeDryRun {
		consequence = "Preview only. No cleanup actions will be applied."
	}
	lines := []string{titleStyle.Render("Review every selected target"), fmt.Sprintf("%s · %d items · %s estimated", m.actionLabel(), len(items), humanBytes(m.selectedTotal())), consequence, ""}
	for i := start; i < end; i++ {
		lines = append(lines, fmt.Sprintf("%3d %10s  %s", i+1, humanBytes(items[i].Size), display(condensePath(items[i].Path))))
	}
	for i := end - start; i < h; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, "", fmt.Sprintf("Showing %d–%d of %d · Eligibility is checked again before each action", min(start+1, len(items)), end, len(items)), "[j/k or PgUp/PgDn] Scroll  [y] Confirm  [n/Esc] Back", display(m.notice))
	return m.frame(lines)
}

func (m *Model) renderCleaning() string {
	pct := 0.0
	if m.cleanProg.Total > 0 {
		pct = float64(m.cleanProg.Completed) / float64(m.cleanProg.Total)
	}
	return m.frame([]string{titleStyle.Render("handbyte") + "  " + m.actionLabel(), "", progressBar(pct, max(4, m.width-10)), fmt.Sprintf("%d / %d actions complete", m.cleanProg.Completed, m.cleanProg.Total), display(m.cleanProg.Current), "", display(m.notice), "[Esc/q] Stop after the current action finishes"})
}

func (m *Model) renderDone() string {
	counts := map[string]int{}
	for _, r := range m.cleanRes {
		counts[r.Status]++
	}
	lines := []string{titleStyle.Render("Cleanup results"), resultSummary(m.cleanRes), fmt.Sprintf("%d previewed · %d moved · %d removed · %d skipped · %d failed · %d cancelled", counts["previewed"], counts["trashed"], counts["removed"], counts["skipped"], counts["failed"], counts["cancelled"]), "Trash retains data. Removed bytes are estimates, not measured reclaimed space."}
	h := max(1, m.height-9)
	start := max(0, m.resultCursor-h+1)
	end := min(len(m.cleanRes), start+h)
	for i := start; i < end; i++ {
		r := m.cleanRes[i]
		line := fmt.Sprintf("%-10s %s", r.Status, display(condensePath(r.Path)))
		if i == m.resultCursor {
			line = rowCursorStyle.Render(m.line(line))
		}
		lines = append(lines, line)
	}
	for i := end - start; i < h; i++ {
		lines = append(lines, "")
	}
	detail := ""
	delta := ""
	actions := "[j/k] Inspect [f] Retry failed [r] Rescan"
	if m.resultCursor < len(m.cleanRes) {
		r := m.cleanRes[m.resultCursor]
		detail = r.Message
		if r.JournalError != "" {
			detail += " Journal error: " + r.JournalError
		}
		if r.FreeDelta != nil {
			delta = fmt.Sprintf("Observed volume free-space delta: %d bytes (includes other activity)", *r.FreeDelta)
		}
		if r.Status == "trashed" {
			detail = strings.TrimSpace(detail + " Trash: " + r.Destination)
			if r.CanRestore() {
				actions += " [u] Restore"
			}
		}
	}
	lines = append(lines, display(detail), delta, display(m.notice), actions, "[Enter/q] Exit · History: handbyte --history")
	return m.frame(lines)
}

func (m *Model) renderHelp() string {
	return m.frame([]string{titleStyle.Render("handbyte help"), "",
		"j/k, arrows      Move through items",
		"Space / a / A    Select item / eligible group / recommendations",
		"c                Clear selection",
		"/                Search names, paths and descriptions",
		"f / s            Change eligibility filter / sort order",
		"p                Add or remove a persistent path protection",
		"e / o            Explore directory / reveal in Finder",
		"Enter            Details / collapse group / enter Explore directory",
		"Backspace / Esc  Previous directory / return from Explore",
		"d                Review all selected actions",
		"y                Confirm on the review screen (Enter does not)",
		"r                Rescan; selection is cleared",
		"q / Ctrl+C       Quit; during cleanup, stop after current action", "",
		"No items start selected. Unknown or active owners stay protected.",
		"Activity age is based on modification times, not last use.",
		"Owner checks are conservative and may protect unrelated projects.", "", "Press any key to return"})
}

func (m *Model) renderDetails() string {
	it := m.currentItem()
	if it == nil {
		return m.frame([]string{"No item selected", "[Esc] Back"})
	}
	blocks := []string{"Name: " + it.Name, "Owner: " + it.Owner, "Path: " + it.Path, "Project: " + it.Project, "Eligibility: " + string(it.Eligibility), "Reason: " + it.Reason, "Effect: " + it.Description, "Scan: " + it.ScanStatus + " " + it.ScanError, "Activity: " + string(it.Activity), "Modified: " + it.ModTime.Format(time.RFC3339), "Rule: " + it.Source, "Size: " + humanBytes(it.Size) + " (estimated allocated bytes)"}
	body := []string{}
	for _, block := range blocks {
		wrapped := lipgloss.NewStyle().Width(max(1, m.width-2)).Render(display(block))
		body = append(body, strings.Split(wrapped, "\n")...)
		body = append(body, "")
	}
	h := max(1, m.height-3)
	start := min(m.detailOffset, max(0, len(body)-h))
	end := min(len(body), start+h)
	lines := []string{titleStyle.Render("Target details")}
	lines = append(lines, body[start:end]...)
	lines = append(lines, "[j/k, PgUp/PgDn] Scroll  [Enter/Esc] Back")
	return m.frame(lines)
}
