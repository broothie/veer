package main

import (
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultSidebarWidth = 35
	minSidebarWidth     = 15
	maxSidebarWidth     = 80
	headerHeight        = 1 // header bar
	statusHeight        = 1 // status bar
	paneBorderSize      = 2
)

type focusArea int

const (
	focusFiles focusArea = iota
	focusCommitMsg
	focusCommits
	focusDiff
)

var (
	styleAdd      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleRem      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleAddLine  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Background(lipgloss.Color("22"))
	styleRemLine  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Background(lipgloss.Color("52"))
	styleGutter   = lipgloss.NewStyle().Background(lipgloss.Color("237"))
	styleFaint    = lipgloss.NewStyle().Faint(true)
	styleBold     = lipgloss.NewStyle().Bold(true)
	styleActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
	styleSelectBG = lipgloss.NewStyle().Background(lipgloss.Color("237"))
	styleBranch   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleSHA      = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleDir      = lipgloss.NewStyle().Faint(true)
	styleStaged   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleHeader   = lipgloss.NewStyle().Faint(true)
	styleFilePath = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("237"))
)

// hunkRef maps a viewport line to a file and hunk index.
type hunkRef struct {
	fileIdx int
	hunkIdx int // -1 for non-hunk lines (headers, separators)
}

type (
	tickMsg       struct{}
	diffResultMsg struct {
		result   *DiffResult
		commits  []CommitInfo
		headSHA  string // SHA used to determine whether commits were refreshed
		repoRoot string
		err      error
	}
	commitDiffMsg struct {
		sha   string
		files []FileDiff
		err   error
	}
)

type model struct {
	cfg            config
	files          []FileDiff
	tree           []treeEntry
	branch         string
	sha            string
	message        string
	cwd            string
	repoRoot       string
	cursor         int
	sidebarOffset  int
	viewport       viewport.Model
	focus          focusArea
	width          int
	height         int
	sidebarWidth   int
	dragging       bool
	err            error
	fetching       bool
	diffGen        uint64    // incremented when files change
	lastBuiltGen   uint64    // gen when diff content was last built
	lastLogSHA     string    // HEAD SHA at last commit-log fetch; skip Log() when unchanged
	fileOffsets    []int     // line offset where each file starts in the viewport
	hunkRefs       []hunkRef // maps viewport lines to file+hunk indices
	commits        []CommitInfo
	commitCursor   int // 0 = "working tree", 1+ = commits[i-1]
	commitOffset   int
	selectedCommit int // -1 = working tree, 0+ = index into commits
	commitMsg      textarea.Model

	// File watcher state.
	watcherCh     <-chan struct{} // receives file change notifications
	watcherClose  func()          // cleanup watcher resources
	watcherReady  bool            // true once watcher is started
	pendingChange bool            // change detected while a fetch was in progress
	lastFetchAt   time.Time       // when the last fetch completed
}

func newModel(cfg config) model {
	cwd, err := os.Getwd()
	if err != nil {
		debugf("newModel: Getwd failed: %v", err)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(cwd, home) {
		cwd = "~" + cwd[len(home):]
	}
	sw := cfg.SidebarWidth
	if sw < minSidebarWidth {
		sw = minSidebarWidth
	} else if sw > maxSidebarWidth {
		sw = maxSidebarWidth
	}
	ti := textarea.New()
	ti.Placeholder = "commit message"
	ti.CharLimit = 500
	ti.SetHeight(3)
	ti.ShowLineNumbers = false
	m := model{
		cfg:            cfg,
		focus:          focusFiles,
		cwd:            cwd,
		width:          80,
		height:         24,
		sidebarWidth:   sw,
		selectedCommit: -1,
		commitMsg:      ti,
	}
	m.viewport = viewport.New(m.vpWidth(), m.paneBodyHeight())
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchCmd(m.cfg, m.lastLogSHA), tickCmd(m.cfg.Interval))
}

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg { return tickMsg{} })
}

func fetchCmd(cfg config, lastLogSHA string) tea.Cmd {
	return func() tea.Msg {
		repo, err := openRepo()
		if err != nil {
			return diffResultMsg{err: err}
		}
		result, err := fetchDiff(repo, cfg)

		// Only re-fetch the commit log when HEAD has moved (new commit, reset, etc.).
		head, headErr := repo.Head()
		var commits []CommitInfo
		if headErr != nil {
			debugf("fetchCmd: Head failed: %v", headErr)
		} else if head.SHA != lastLogSHA {
			var logErr error
			commits, logErr = repo.Log(50)
			if logErr != nil {
				debugf("fetchCmd: Log failed: %v", logErr)
			}
		}

		return diffResultMsg{
			result:   result,
			commits:  commits,
			headSHA:  head.SHA,
			repoRoot: repo.wt.Filesystem.Root(),
			err:      err,
		}
	}
}

func waitForChange(ch <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		if _, ok := <-ch; !ok {
			return nil
		}
		return filesChangedMsg{}
	}
}

func commitDiffCmd(sha string) tea.Cmd {
	return func() tea.Msg {
		repo, err := openRepo()
		if err != nil {
			return commitDiffMsg{sha, nil, err}
		}
		files, err := repo.DiffCommit(sha)
		return commitDiffMsg{sha, files, err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()

	case filesChangedMsg:
		cmds := []tea.Cmd{waitForChange(m.watcherCh)}
		if m.fetching {
			m.pendingChange = true
			return m, tea.Batch(cmds...)
		}
		m.fetching = true
		m.pendingChange = false
		cmds = append(cmds, fetchCmd(m.cfg, m.lastLogSHA))
		return m, tea.Batch(cmds...)

	case tickMsg:
		cmds := []tea.Cmd{tickCmd(m.cfg.Interval)}
		if m.fetching {
			return m, tea.Batch(cmds...)
		}
		// When the watcher is active, tick is a fallback — only fetch
		// if enough time has passed since the last fetch.
		if m.watcherReady && time.Since(m.lastFetchAt) < m.cfg.Interval*5 {
			return m, tea.Batch(cmds...)
		}
		m.fetching = true
		cmds = append(cmds, fetchCmd(m.cfg, m.lastLogSHA))
		return m, tea.Batch(cmds...)

	case diffResultMsg:
		m.fetching = false
		m.lastFetchAt = time.Now()
		m.err = msg.err

		// Start file watcher on first successful fetch.
		var cmds []tea.Cmd
		if !m.watcherReady && msg.repoRoot != "" {
			ch, cleanup := startWatcher(msg.repoRoot, m.cfg.Debounce, m.cfg.SkipDirs)
			if ch != nil {
				m.watcherCh = ch
				m.watcherClose = cleanup
				m.watcherReady = true
				cmds = append(cmds, waitForChange(ch))
			}
		}

		if msg.repoRoot != "" {
			m.repoRoot = msg.repoRoot
		}
		if msg.commits != nil {
			m.commits = msg.commits
			m.lastLogSHA = msg.headSHA
		}
		// Only update file data if in working tree mode.
		if msg.result != nil && m.selectedCommit == -1 {
			m.branch = msg.result.Branch
			m.sha = msg.result.SHA
			m.message = msg.result.Message

			prevYOffset := m.viewport.YOffset

			m.files = msg.result.Files
			m.tree = buildTree(m.files)

			m.diffGen++
			m.rebuildDiffContent()

			m.viewport.SetYOffset(prevYOffset)
			m.syncCursorToScroll()
		}

		// Re-fetch immediately if changes arrived during this fetch.
		if m.pendingChange {
			m.pendingChange = false
			m.fetching = true
			cmds = append(cmds, fetchCmd(m.cfg, m.lastLogSHA))
		}

		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}

	case stageResultMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		m.fetching = true
		return m, fetchCmd(m.cfg, m.lastLogSHA)

	case commitResultMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.commitMsg.Reset()
			m.commitMsg.Blur()
			m.focus = focusFiles
		}
		m.fetching = true
		return m, fetchCmd(m.cfg, m.lastLogSHA)

	case commitDiffMsg:
		// Only apply if this is still the selected commit.
		if m.selectedCommit >= 0 && m.selectedCommit < len(m.commits) &&
			m.commits[m.selectedCommit].FullSHA == msg.sha {
			m.err = msg.err
			m.files = msg.files
			m.tree = buildTree(m.files)
			m.diffGen++
			m.rebuildDiffContent()
			m.viewport.GotoTop()
			m.syncCursorToScroll()
		}

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	return m, nil
}

func (m model) mainHeight() int {
	return max(1, m.height-headerHeight-statusHeight)
}

func (m model) paneBodyHeight() int {
	return max(1, m.mainHeight()-paneBorderSize)
}

func (m model) sidebarBodyHeight() int {
	return max(1, m.mainHeight()-(2*paneBorderSize))
}

func (m model) sidebarPaneWidth() int {
	return m.sidebarWidth + paneBorderSize
}

func (m model) vpWidth() int {
	return max(1, m.width-m.sidebarPaneWidth()-paneBorderSize)
}

func (m *model) recalcLayout() {
	prevYOffset := m.viewport.YOffset
	m.viewport = viewport.New(m.vpWidth(), m.paneBodyHeight())
	m.diffGen++
	m.lastBuiltGen = 0
	m.rebuildDiffContent()
	m.viewport.SetYOffset(prevYOffset)
	m.syncCursorToScroll()
}

// setCursor moves the file cursor and scrolls the viewport to that file.
func (m *model) setCursor(idx int) {
	if idx < 0 || idx >= len(m.files) || idx == m.cursor {
		return
	}
	m.cursor = idx
	if idx < len(m.fileOffsets) {
		m.viewport.SetYOffset(m.fileOffsets[idx])
	}
}

// syncCursorToScroll updates the sidebar cursor to match the current scroll position.
func (m *model) syncCursorToScroll() {
	if len(m.fileOffsets) == 0 {
		return
	}
	yOff := m.viewport.YOffset
	for i := len(m.fileOffsets) - 1; i >= 0; i-- {
		if yOff >= m.fileOffsets[i] {
			m.cursor = i
			return
		}
	}
	m.cursor = 0
}

// rebuildDiffContent rebuilds viewport content only if the generation changed.
func (m *model) rebuildDiffContent() {
	if m.lastBuiltGen == m.diffGen && m.diffGen > 0 {
		return
	}
	content := m.buildDiffContent()
	m.viewport.SetContent(content)
	m.lastBuiltGen = m.diffGen
}

func (m model) inSidebar() bool {
	return m.focus == focusFiles || m.focus == focusCommitMsg || m.focus == focusCommits
}

func (m model) isWorkingTree() bool {
	return m.selectedCommit == -1 && m.cfg.Ref == ""
}

func (m model) hasStaged() bool {
	for _, f := range m.files {
		if f.Staged {
			return true
		}
	}
	return false
}

func (m model) commitMsgVisible() bool {
	return m.isWorkingTree() && (m.hasStaged() || m.focus == focusCommitMsg)
}

func (m *model) toggleStage() tea.Cmd {
	if !m.isWorkingTree() || m.repoRoot == "" {
		return nil
	}
	switch m.focus {
	case focusFiles:
		return m.toggleStageFile()
	case focusDiff:
		return m.stageCurrentHunk()
	}
	return nil
}

func (m *model) toggleStageFile() tea.Cmd {
	return m.stageFileAt(m.cursor)
}

func (m *model) stageFileAt(idx int) tea.Cmd {
	if !m.isWorkingTree() || m.repoRoot == "" {
		return nil
	}
	if idx < 0 || idx >= len(m.files) {
		return nil
	}
	f := m.files[idx]
	if f.Unstaged {
		return stageFileCmd(m.repoRoot, f.Path)
	}
	if f.Staged {
		return unstageFileCmd(m.repoRoot, f.Path)
	}
	return nil
}

func (m *model) stageHunkAtY(y int) tea.Cmd {
	if !m.isWorkingTree() || m.repoRoot == "" {
		return nil
	}
	vpLine := (y - headerHeight) + m.viewport.YOffset
	if vpLine < 0 || vpLine >= len(m.hunkRefs) {
		return nil
	}
	ref := m.hunkRefs[vpLine]
	if ref.hunkIdx < 0 || ref.fileIdx < 0 {
		return nil
	}
	f := m.files[ref.fileIdx]
	if ref.hunkIdx >= len(f.Hunks) {
		return nil
	}
	hunk := f.Hunks[ref.hunkIdx]
	if hunk.Section == "staged" {
		return unstageHunkCmd(m.repoRoot, f.Path, hunk)
	}
	return stageHunkCmd(m.repoRoot, f.Path, hunk)
}

func (m *model) stageCurrentHunk() tea.Cmd {
	yOff := m.viewport.YOffset
	if yOff < 0 || yOff >= len(m.hunkRefs) {
		return nil
	}

	// Find the hunk at or near the top of the viewport.
	ref := m.hunkRefs[yOff]
	if ref.hunkIdx < 0 {
		// Look forward for the nearest content line.
		for y := yOff + 1; y < len(m.hunkRefs); y++ {
			if m.hunkRefs[y].hunkIdx >= 0 {
				ref = m.hunkRefs[y]
				break
			}
		}
	}
	if ref.hunkIdx < 0 || ref.fileIdx < 0 {
		return nil
	}

	f := m.files[ref.fileIdx]
	if ref.hunkIdx >= len(f.Hunks) {
		return nil
	}

	hunk := f.Hunks[ref.hunkIdx]
	if hunk.Section == "staged" {
		return unstageHunkCmd(m.repoRoot, f.Path, hunk)
	}

	return stageHunkCmd(m.repoRoot, f.Path, hunk)
}

func (m *model) unstageAll() tea.Cmd {
	if !m.isWorkingTree() || m.repoRoot == "" || !m.hasStaged() {
		return nil
	}
	return unstageAllCmd(m.repoRoot)
}

func (m *model) focusCommitMessage() tea.Cmd {
	if !m.isWorkingTree() {
		return nil
	}
	m.focus = focusCommitMsg
	return m.commitMsg.Focus()
}

func (m *model) focusHistoryPane() tea.Cmd {
	if len(m.commits) > 0 {
		if m.focus == focusCommitMsg {
			m.commitMsg.Blur()
		}
		m.focus = focusCommits
		return nil
	}
	return m.focusCommitMessage()
}

func (m *model) focusPane(key string) tea.Cmd {
	switch key {
	case "1":
		if m.focus == focusCommitMsg {
			m.commitMsg.Blur()
		}
		m.focus = focusFiles
	case "2":
		return m.focusHistoryPane()
	case "3":
		if len(m.files) == 0 {
			return nil
		}
		if m.focus == focusCommitMsg {
			m.commitMsg.Blur()
		}
		m.focus = focusDiff
	}
	return nil
}

func (m model) submitCommit() (tea.Model, tea.Cmd) {
	msg := strings.TrimSpace(m.commitMsg.Value())
	if msg == "" || m.repoRoot == "" {
		return m, nil
	}
	m.commitMsg.Reset()
	m.commitMsg.Blur()
	m.focus = focusFiles
	return m, commitStagedCmd(m.repoRoot, msg)
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Commit message input handles its own keys.
	if m.focus == focusCommitMsg {
		switch msg.String() {
		case "1", "2", "3":
			return m, m.focusPane(msg.String())
		case "esc":
			m.commitMsg.Blur()
			m.commitMsg.Reset()
			m.focus = focusFiles
			return m, nil
		case "ctrl+d":
			return m.submitCommit()
		case "tab":
			m.commitMsg.Blur()
			m.cycleTab(1)
			return m, nil
		case "shift+tab":
			m.commitMsg.Blur()
			m.cycleTab(-1)
			return m, nil
		default:
			var cmd tea.Cmd
			m.commitMsg, cmd = m.commitMsg.Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "1", "2", "3":
		return m, m.focusPane(msg.String())
	case "q", "ctrl+c":
		if m.watcherClose != nil {
			m.watcherClose()
		}
		return m, tea.Quit
	case "tab":
		m.cycleTab(1)
	case "shift+tab":
		m.cycleTab(-1)
	case "j", "down":
		return m, m.keyDown()
	case "k", "up":
		return m, m.keyUp()
	case "g":
		return m, m.keyTop()
	case "G":
		return m, m.keyBottom()
	case "ctrl+f":
		m.diffScroll(func(v *viewport.Model) { v.ViewDown() })
	case "ctrl+b":
		m.diffScroll(func(v *viewport.Model) { v.ViewUp() })
	case "enter":
		return m.keyOpen()
	case "s":
		return m, m.toggleStage()
	case "u":
		return m, m.unstageAll()
	case "c":
		return m, m.focusCommitMessage()
	}
	return m, nil
}

// cycleTab advances focus forward (dir=1) or backward (dir=-1).
// Order: focusFiles → focusCommitMsg → focusCommits → focusDiff → focusFiles
func (m *model) cycleTab(dir int) {
	hasCommits := len(m.commits) > 0
	hasFiles := len(m.files) > 0
	hasCommitMsg := m.commitMsgVisible()

	// Build ordered list of available focus areas.
	areas := []focusArea{focusFiles}
	if hasCommitMsg {
		areas = append(areas, focusCommitMsg)
	}
	if hasCommits {
		areas = append(areas, focusCommits)
	}
	if hasFiles {
		areas = append(areas, focusDiff)
	}
	if len(areas) <= 1 {
		return
	}

	cur := 0
	for i, a := range areas {
		if a == m.focus {
			cur = i
			break
		}
	}
	next := (cur + dir + len(areas)) % len(areas)
	m.focus = areas[next]
}

func (m *model) keyDown() tea.Cmd {
	switch m.focus {
	case focusFiles:
		m.setCursor(m.cursor + 1)
	case focusCommits:
		total := len(m.commits) + 1
		if m.commitCursor < total-1 {
			m.commitCursor++
			return m.applyCommitCursor()
		}
	case focusDiff:
		m.viewport.LineDown(1)
		m.syncCursorToScroll()
	}
	return nil
}

func (m *model) keyUp() tea.Cmd {
	switch m.focus {
	case focusFiles:
		m.setCursor(m.cursor - 1)
	case focusCommits:
		if m.commitCursor > 0 {
			m.commitCursor--
			return m.applyCommitCursor()
		}
	case focusDiff:
		m.viewport.LineUp(1)
		m.syncCursorToScroll()
	}
	return nil
}

func (m *model) keyTop() tea.Cmd {
	switch m.focus {
	case focusFiles:
		m.setCursor(0)
	case focusCommits:
		m.commitCursor = 0
		return m.applyCommitCursor()
	case focusDiff:
		m.viewport.GotoTop()
		m.syncCursorToScroll()
	}
	return nil
}

func (m *model) keyBottom() tea.Cmd {
	switch m.focus {
	case focusFiles:
		m.setCursor(len(m.files) - 1)
	case focusCommits:
		m.commitCursor = len(m.commits)
		return m.applyCommitCursor()
	case focusDiff:
		m.viewport.GotoBottom()
		m.syncCursorToScroll()
	}
	return nil
}

func (m *model) diffScroll(fn func(*viewport.Model)) {
	if m.focus == focusDiff {
		fn(&m.viewport)
		m.syncCursorToScroll()
	}
}

func (m model) keyOpen() (tea.Model, tea.Cmd) {
	switch m.focus {
	case focusFiles:
		if len(m.files) > 0 {
			m.focus = focusDiff
		}
	case focusCommits:
		return m.selectCommit()
	}
	return m, nil
}

// applyCommitCursor loads the diff for the current commit cursor position immediately,
// without changing focus. Used for instant preview during j/k/scroll navigation.
func (m *model) applyCommitCursor() tea.Cmd {
	if m.commitCursor == 0 {
		if m.selectedCommit != -1 {
			m.selectedCommit = -1
		}
		return nil
	}
	idx := m.commitCursor - 1
	if idx >= len(m.commits) {
		return nil
	}
	c := m.commits[idx]
	m.selectedCommit = idx
	m.branch = ""
	m.sha = c.SHA
	m.message = c.Message
	return commitDiffCmd(c.FullSHA)
}

// selectCommit handles enter in the commit list.
func (m model) selectCommit() (tea.Model, tea.Cmd) {
	if m.commitCursor == 0 {
		// Select "working tree".
		if m.selectedCommit != -1 {
			m.selectedCommit = -1
			m.focus = focusFiles
			// Working tree data will be refreshed on next tick.
		}
		return m, nil
	}

	idx := m.commitCursor - 1
	if idx >= len(m.commits) {
		return m, nil
	}

	c := m.commits[idx]
	m.selectedCommit = idx
	m.branch = ""
	m.sha = c.SHA
	m.message = c.Message
	m.focus = focusFiles
	return m, commitDiffCmd(c.FullSHA)
}

func (m model) sidebarBorderX() int {
	return m.sidebarPaneWidth() - 1
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonLeft:
		return m.handleMouseLeft(msg)
	case tea.MouseButtonNone:
		if msg.Action == tea.MouseActionRelease {
			m.dragging = false
		}
	case tea.MouseButtonWheelUp:
		return m, m.handleMouseScroll(msg, -1)
	case tea.MouseButtonWheelDown:
		return m, m.handleMouseScroll(msg, 1)
	}
	return m, nil
}

func (m model) handleMouseLeft(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	borderX := m.sidebarBorderX()

	switch msg.Action {
	case tea.MouseActionPress:
		if msg.X >= borderX-1 && msg.X <= borderX+1 {
			m.dragging = true
			return m, nil
		}
		return m.handleMouseClick(msg, borderX)

	case tea.MouseActionMotion:
		if m.dragging {
			newWidth := max(minSidebarWidth, min(maxSidebarWidth, msg.X-2))
			if newWidth != m.sidebarWidth {
				m.sidebarWidth = newWidth
				m.recalcLayout()
			}
		}

	case tea.MouseActionRelease:
		m.dragging = false
	}
	return m, nil
}

func (m model) handleMouseClick(msg tea.MouseMsg, borderX int) (tea.Model, tea.Cmd) {
	row := msg.Y - headerHeight
	if row < 0 || row >= m.mainHeight() {
		return m, nil
	}

	if msg.X >= m.sidebarPaneWidth() {
		bodyRow := row - 1
		m.focus = focusDiff
		if bodyRow < 0 || bodyRow >= m.paneBodyHeight() {
			return m, nil
		}
		// Click in diff area — stage the clicked hunk.
		if cmd := m.stageHunkAtY(msg.Y); cmd != nil {
			return m, cmd
		}
		return m, nil
	}

	fileH, msgH, commitH := m.sidebarSplit()
	filePaneH := fileH + paneBorderSize
	if row < filePaneH {
		if row == 0 || row == filePaneH-1 {
			m.focus = focusFiles
			return m, nil
		}

		bodyRow := row - 1
		treeRow := m.sidebarOffset + bodyRow
		if treeRow >= 0 && treeRow < len(m.tree) {
			entry := m.tree[treeRow]
			if entry.fileIdx >= 0 {
				// Click on status indicator (right side) toggles staging.
				innerX := msg.X - 1
				if innerX >= m.sidebarWidth-5 && innerX < m.sidebarWidth {
					if cmd := m.stageFileAt(entry.fileIdx); cmd != nil {
						return m, cmd
					}
					return m, nil
				}
				m.setCursor(entry.fileIdx)
				m.focus = focusDiff
			}
		}
		return m, nil
	}

	historyRow := row - filePaneH
	historyPaneH := msgH + commitH + paneBorderSize
	if historyRow < 0 || historyRow >= historyPaneH {
		return m, nil
	}
	if historyRow == 0 || historyRow == historyPaneH-1 {
		m.focus = focusCommits
		return m, nil
	}

	bodyRow := historyRow - 1
	if msgH > 0 && bodyRow < msgH {
		m.focus = focusCommitMsg
		return m, m.commitMsg.Focus()
	}
	return m.handleCommitClick(bodyRow - msgH)
}

func (m model) handleCommitClick(row int) (tea.Model, tea.Cmd) {
	commitRow := m.commitOffset + row
	total := len(m.commits) + 1
	if commitRow >= 0 && commitRow < total {
		m.commitCursor = commitRow
		m.focus = focusCommits
		return m.selectCommit()
	}
	return m, nil
}

func (m *model) handleMouseScroll(msg tea.MouseMsg, dir int) tea.Cmd {
	borderX := m.sidebarBorderX()
	fileH, msgH, commitH := m.sidebarSplit()

	row := msg.Y - headerHeight
	if row < 0 || row >= m.mainHeight() {
		return nil
	}

	if msg.X >= m.sidebarPaneWidth() || msg.X > borderX {
		bodyRow := row - 1
		if bodyRow < 0 || bodyRow >= m.paneBodyHeight() {
			return nil
		}
		if dir > 0 {
			m.viewport.LineDown(3)
		} else {
			m.viewport.LineUp(3)
		}
		m.syncCursorToScroll()
		return nil
	}

	filePaneH := fileH + paneBorderSize
	if row < filePaneH {
		if row == 0 || row == filePaneH-1 {
			return nil
		}
		m.scrollFileTree(dir)
		return nil
	}

	historyRow := row - filePaneH
	historyPaneH := msgH + commitH + paneBorderSize
	if historyRow < 0 || historyRow >= historyPaneH || historyRow == 0 || historyRow == historyPaneH-1 {
		return nil
	}
	if historyRow-1 >= msgH {
		return m.scrollCommitList(dir)
	}
	return nil
}

func (m *model) scrollCommitList(dir int) tea.Cmd {
	total := len(m.commits) + 1
	commitH := m.commitListHeight()
	if dir > 0 {
		if m.commitCursor < total-1 {
			m.commitCursor++
		}
		maxOff := max(0, total-commitH)
		m.commitOffset = min(m.commitOffset+1, maxOff)
	} else {
		if m.commitCursor > 0 {
			m.commitCursor--
		}
		m.commitOffset = max(0, m.commitOffset-1)
	}
	return m.applyCommitCursor()
}

func (m *model) scrollFileTree(dir int) {
	if dir > 0 {
		m.setCursor(m.cursor + 1)
	} else {
		m.setCursor(m.cursor - 1)
	}
}

// sidebarSplit returns (fileTreeHeight, commitMsgHeight, commitListHeight).
func (m model) sidebarSplit() (int, int, int) {
	mainH := m.sidebarBodyHeight()
	var msgH int
	if m.commitMsgVisible() {
		msgH = 4 // label + 3 input lines
	}
	commitH := m.commitListHeight()
	fileH := mainH - commitH - msgH
	if fileH < 1 {
		deficit := 1 - fileH
		if commitH > 1 {
			reduce := min(deficit, commitH-1)
			commitH -= reduce
			deficit -= reduce
		}
		if deficit > 0 && msgH > 0 {
			reduce := min(deficit, msgH)
			msgH -= reduce
		}
		fileH = mainH - commitH - msgH
	}
	if fileH < 1 {
		fileH = 1
	}
	return fileH, msgH, commitH
}

func (m model) commitListHeight() int {
	mainH := m.sidebarBodyHeight()
	total := len(m.commits) + 1 // +1 for working tree entry
	h := min(total, mainH/3)
	return max(h, 3) // at least 3 rows
}

func (m model) diffPaneTitle() string {
	switch {
	case m.selectedCommit >= 0 && m.sha != "":
		return " 3. diff commit " + m.sha + " "
	case m.cfg.Ref != "":
		return " 3. diff vs " + m.cfg.Ref + " "
	default:
		return " 3. diff vs HEAD "
	}
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}

	header := m.renderHeader()

	mainH := m.mainHeight()
	diffBodyH := m.paneBodyHeight()

	fileH, msgH, commitH := m.sidebarSplit()

	// Keep cursor's tree row visible in file tree scroll region.
	cursorRow := m.cursorTreeRow()
	if cursorRow < m.sidebarOffset {
		m.sidebarOffset = cursorRow
	} else if cursorRow >= m.sidebarOffset+fileH {
		m.sidebarOffset = cursorRow - fileH + 1
	}

	// Keep commit cursor visible.
	visibleCommits := commitH
	if m.commitCursor < m.commitOffset {
		m.commitOffset = m.commitCursor
	} else if m.commitCursor >= m.commitOffset+visibleCommits {
		m.commitOffset = m.commitCursor - visibleCommits + 1
	}

	content := m.viewport.View()

	// Diff scrollbar.
	totalDiffLines := m.viewport.TotalLineCount()
	diffScrollbar := renderScrollbar(diffBodyH, totalDiffLines, m.viewport.YOffset, m.focus == focusDiff)

	fileScrollbar := renderScrollbar(fileH, len(m.tree), m.sidebarOffset, m.focus == focusFiles)
	filePane := renderPane(" 1. files ", m.renderFileTree(fileH), fileScrollbar, m.sidebarPaneWidth(), fileH+paneBorderSize, m.focus == focusFiles)

	commitScrollbar := renderScrollbar(commitH, len(m.commits)+1, m.commitOffset, m.focus == focusCommits)
	historyScrollbar := commitScrollbar
	if msgH > 0 {
		historyScrollbar = joinOverlay(msgH, commitScrollbar)
	}
	historyFocused := m.focus == focusCommitMsg || m.focus == focusCommits
	historyPane := renderPane(" 2. history ", m.renderHistoryBody(msgH, commitH), historyScrollbar, m.sidebarPaneWidth(), msgH+commitH+paneBorderSize, historyFocused)

	diffPane := renderPane(m.diffPaneTitle(), content, diffScrollbar, m.width-m.sidebarPaneWidth(), mainH, m.focus == focusDiff)
	sidebarColumn := lipgloss.JoinVertical(lipgloss.Left, filePane, historyPane)
	main := lipgloss.JoinHorizontal(lipgloss.Top, sidebarColumn, diffPane)
	status := m.renderStatus()

	return lipgloss.JoinVertical(lipgloss.Left, header, main, status)
}
