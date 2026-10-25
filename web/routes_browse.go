package web

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/andreassag/morphic/internal/shared"
	"github.com/gin-gonic/gin"
)

func registerBrowseRoutes(r *gin.Engine) {
	r.GET("/api/browse", handleBrowseDirectory)
	r.POST("/api/browse/resolve", handleBrowseResolve)
}

var (
	// windowsDrivePathRe matches host paths such as D:\Photos or d:/Photos.
	windowsDrivePathRe = regexp.MustCompile(`^([A-Za-z]):(?:[\\/](.*))?$`)
	// wslUNCPathRe matches \\wsl$\Distro\home\x and \\wsl.localhost\Distro\home\x.
	wslUNCPathRe = regexp.MustCompile(`(?i)^[\\/]{2}wsl(?:\$|\.localhost)[\\/][^\\/]+(?:[\\/](.*))?$`)
)

// translateHostPath converts paths copied from a Windows file explorer (e.g. D:\Photos or
// \\wsl$\Ubuntu\home\me) into their equivalent on a Linux/macOS runner (/mnt/d/Photos, /home/me).
// This lets users paste paths from their client OS even when Morphic runs in Docker/WSL.
func translateHostPath(p string) string {
	if runtime.GOOS == "windows" {
		return p
	}
	if m := wslUNCPathRe.FindStringSubmatch(p); m != nil {
		return "/" + strings.ReplaceAll(m[1], `\`, "/")
	}
	if m := windowsDrivePathRe.FindStringSubmatch(p); m != nil {
		rest := strings.ReplaceAll(m[2], `\`, "/")
		return "/mnt/" + strings.ToLower(m[1]) + "/" + rest
	}
	return p
}

// expandPath cleans the path and expands a leading tilde (~) to the user's home directory.
// Windows-style host paths are translated for non-Windows runners.
func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = translateHostPath(p)
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Clean(home)
		}
	} else if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Clean(filepath.Join(home, p[2:]))
		}
	}
	return filepath.Clean(p)
}

// validateBrowsePath validates a path for directory browsing, allowing the filesystem root ("/")
// while still rejecting protected OS system subdirectories (/etc, /proc, /sys, etc.).
func validateBrowsePath(raw string) (string, error) {
	expanded := expandPath(raw)
	if expanded == "" {
		return "", shared.ErrEmptyPath
	}
	if expanded == "/" || expanded == `\` {
		return "/", nil
	}
	return shared.ValidateSafePath(expanded)
}

// windowsDrives returns the root paths (e.g. C:\) of all available drive letters.
func windowsDrives() []string {
	var drives []string
	for l := 'A'; l <= 'Z'; l++ {
		root := string(l) + `:\`
		if _, err := os.Stat(root); err == nil {
			drives = append(drives, root)
		}
	}
	return drives
}

func isDirPath(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// subDirs returns the visible (non-hidden) subdirectory paths of dir, following directory symlinks.
func subDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		if e.IsDir() || (e.Type()&os.ModeSymlink != 0 && isDirPath(full)) {
			out = append(out, full)
		}
	}
	return out
}

// containerInternalDirs are top-level directories that never contain user media.
var containerInternalDirs = map[string]bool{
	"app": true, "tmp": true, "var": true, "usr": true,
}

// detectBrowseRoots returns the available top-level mount/navigation roots for quick access.
func detectBrowseRoots() []map[string]string {
	var roots []map[string]string
	seen := make(map[string]bool)

	addRoot := func(label, p string) {
		clean := filepath.Clean(p)
		if seen[clean] || !isDirPath(clean) {
			return
		}
		seen[clean] = true
		roots = append(roots, map[string]string{"label": label, "path": clean})
	}

	if envDir := os.Getenv("MORPHIC_DEFAULT_DIR"); envDir != "" {
		addRoot("Default", envDir)
	}

	if runtime.GOOS == "windows" {
		for _, d := range windowsDrives() {
			addRoot(strings.TrimSuffix(d, `\`), d)
		}
	} else {
		addRoot("/media", "/media")
		for _, d := range subDirs("/mnt") {
			addRoot(d, d)
		}
		for _, d := range subDirs("/Volumes") {
			addRoot(d, d)
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		addRoot("~", home)
	}
	addRoot("All (/)", "/")
	return roots
}

// defaultBrowseStartPath chooses the most useful starting directory when no path is specified.
func defaultBrowseStartPath() string {
	if envDir := os.Getenv("MORPHIC_DEFAULT_DIR"); envDir != "" && isDirPath(envDir) {
		return filepath.Clean(envDir)
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/"
	}
	if runtime.GOOS == "windows" {
		return home
	}

	// In Docker, home (/home/appuser) is empty while media is mounted at /media or /mnt/*.
	if len(subDirs(home)) == 0 {
		for _, candidate := range []string{"/mnt/d/storage", "/mnt/d", "/media"} {
			if entries, err := os.ReadDir(candidate); err == nil && len(entries) > 0 {
				return candidate
			}
		}
		return "/"
	}
	return home
}

// browseParent returns the parent directory of dir, or nil when dir is the top of the tree.
func browseParent(dir string) interface{} {
	parent := filepath.Dir(dir)
	if parent == dir {
		if runtime.GOOS == "windows" {
			return "/" // drive root -> virtual drive list
		}
		return nil
	}
	return parent
}

// handleBrowseDirectory lists directories for the in-page folder browser.
func handleBrowseDirectory(c *gin.Context) {
	rawPath := strings.TrimSpace(c.Query("path"))
	if rawPath == "" {
		rawPath = defaultBrowseStartPath()
	}

	safePath, err := validateBrowsePath(rawPath)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PATH", err.Error())
		return
	}

	type dirEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}

	// Windows runner: "/" is a virtual root listing all drives.
	if runtime.GOOS == "windows" && safePath == "/" {
		dirs := []dirEntry{}
		for _, d := range windowsDrives() {
			dirs = append(dirs, dirEntry{Name: strings.TrimSuffix(d, `\`), Path: d, Type: "directory"})
		}
		c.JSON(http.StatusOK, gin.H{
			"current": "/",
			"parent":  nil,
			"entries": dirs,
			"exists":  true,
			"roots":   detectBrowseRoots(),
		})
		return
	}

	pathExists := true
	browseDir := safePath
	filterPrefix := ""

	if !isDirPath(safePath) {
		pathExists = false
		// If path doesn't exist as a directory, use its parent for autocomplete matching
		parentSafe, pErr := validateBrowsePath(filepath.Dir(safePath))
		if pErr != nil || !isDirPath(parentSafe) {
			respondError(c, http.StatusBadRequest, "NOT_A_DIRECTORY", "Not a directory")
			return
		}
		browseDir = parentSafe
		filterPrefix = strings.ToLower(filepath.Base(safePath))
	}

	showHidden := c.Query("hidden") == "1"

	entries, err := os.ReadDir(browseDir)
	if err != nil {
		// Report unreadable folders (e.g. permission denied) inline so the UI can keep navigating.
		c.JSON(http.StatusOK, gin.H{
			"current": browseDir,
			"parent":  browseParent(browseDir),
			"entries": []dirEntry{},
			"exists":  pathExists,
			"roots":   detectBrowseRoots(),
			"error":   "Cannot read this folder: " + err.Error(),
		})
		return
	}

	dirs := []dirEntry{}
	for _, e := range entries {
		name := e.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if filterPrefix != "" && !strings.HasPrefix(strings.ToLower(name), filterPrefix) {
			continue
		}
		full := filepath.Join(browseDir, name)
		// Hide protected OS system directories and container internals when listing the root
		if shared.IsProtectedSystemPath(full) {
			continue
		}
		if browseDir == "/" && containerInternalDirs[name] {
			continue
		}
		isDirEntry := e.IsDir() || (e.Type()&os.ModeSymlink != 0 && isDirPath(full))
		if isDirEntry {
			dirs = append(dirs, dirEntry{Name: name, Path: full, Type: "directory"})
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	c.JSON(http.StatusOK, gin.H{
		"current": browseDir,
		"parent":  browseParent(browseDir),
		"entries": dirs,
		"exists":  pathExists,
		"roots":   detectBrowseRoots(),
	})
}

// resolveSearchRoots returns the ordered list of directories the resolver should search.
func resolveSearchRoots(hint string) []string {
	var roots []string
	seen := make(map[string]bool)
	add := func(p string) {
		if p == "" {
			return
		}
		clean := filepath.Clean(p)
		if seen[clean] || shared.IsProtectedSystemPath(clean) || !isDirPath(clean) {
			return
		}
		seen[clean] = true
		roots = append(roots, clean)
	}

	if hint != "" {
		h := expandPath(hint)
		add(h)
		add(filepath.Dir(h))
	}

	if runtime.GOOS == "windows" {
		for _, d := range windowsDrives() {
			add(d)
		}
	} else {
		for _, d := range subDirs("/mnt") {
			add(d)
		}
		for _, d := range subDirs("/Volumes") {
			add(d)
		}
		add("/media")
		add("/mnt")
		if home, err := os.UserHomeDir(); err == nil {
			add(home)
		}
		for _, d := range subDirs("/") {
			if containerInternalDirs[filepath.Base(d)] {
				continue
			}
			add(d)
		}
	}
	return roots
}

type resolveCandidate struct {
	Path            string `json:"path"`
	Score           int    `json:"score"`
	MatchedChildren int    `json:"matched_children"`
}

// searchFoldersByName walks the given roots breadth-first (shallowest first) looking for
// directories whose name equals name, ranking them by how many of the picker's child entry
// names they contain. The search stops at the context deadline.
func searchFoldersByName(ctx context.Context, roots []string, name string, childSet map[string]bool) (cands []resolveCandidate, timedOut bool) {
	skipNames := map[string]bool{
		"node_modules": true, "$recycle.bin": true, "system volume information": true,
		"windows": true, "program files": true, "program files (x86)": true,
		"programdata": true, "appdata": true, "lost+found": true, ".morphic_trash": true,
	}

	type item struct {
		path  string
		depth int
	}
	const maxDepth = 16
	const maxVisited = 400000

	queue := make([]item, 0, len(roots))
	visited := make(map[string]bool)
	for _, r := range roots {
		visited[r] = true
		queue = append(queue, item{r, 0})
	}

	visitedCount := 0
	var firstPerfectAt time.Time
	for len(queue) > 0 && visitedCount < maxVisited {
		if ctx.Err() != nil {
			return cands, true
		}
		// Once a perfect match exists, only keep searching briefly to detect look-alike folders.
		if !firstPerfectAt.IsZero() && time.Since(firstPerfectAt) > 2*time.Second {
			return cands, false
		}
		curr := queue[0]
		queue = queue[1:]
		visitedCount++

		entries, err := os.ReadDir(curr.path)
		if err != nil {
			continue
		}

		base := strings.ToLower(filepath.Base(curr.path))
		nameMatches := name != "" && base == name
		matched := 0
		if len(childSet) > 0 {
			for _, e := range entries {
				if childSet[strings.ToLower(e.Name())] {
					matched++
				}
			}
		}

		score := -1
		switch {
		case nameMatches && len(childSet) > 0 && matched == len(childSet):
			score = 100 + matched
		case nameMatches && len(childSet) > 0:
			score = 10 + matched*10
		case nameMatches && len(entries) == 0:
			score = 30
		case nameMatches:
			score = 10
		case curr.depth == 0 && len(childSet) >= 2 && matched == len(childSet):
			// A mount root whose host-side name differs from its container-side name
			score = 60
		}
		if score >= 0 {
			cands = append(cands, resolveCandidate{Path: curr.path, Score: score, MatchedChildren: matched})
			// A perfect match on a non-trivial child set is conclusive: stop searching.
			if score >= 100 {
				if len(childSet) >= 2 {
					return cands, false
				}
				if firstPerfectAt.IsZero() {
					firstPerfectAt = time.Now()
				}
			}
		}

		if curr.depth >= maxDepth {
			continue
		}
		for _, e := range entries {
			n := e.Name()
			if strings.HasPrefix(n, ".") || skipNames[strings.ToLower(n)] {
				continue
			}
			full := filepath.Join(curr.path, n)
			if !e.IsDir() || visited[full] || shared.IsProtectedSystemPath(full) {
				continue
			}
			visited[full] = true
			queue = append(queue, item{full, curr.depth + 1})
		}
	}
	return cands, false
}

// handleBrowseResolve maps a folder chosen with the browser's native directory picker
// (showDirectoryPicker, which only reveals the folder's name and its children's names) to its
// real path on the server by searching all mounted/available roots.
func handleBrowseResolve(c *gin.Context) {
	var req struct {
		Name     string   `json:"name"`
		Children []string `json:"children"`
		Hint     string   `json:"hint"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	name := strings.ToLower(strings.TrimRight(strings.TrimSpace(req.Name), `/\:`))
	childSet := make(map[string]bool, len(req.Children))
	for _, ch := range req.Children {
		if ch = strings.ToLower(strings.TrimSpace(ch)); ch != "" {
			childSet[ch] = true
		}
	}
	if name == "" && len(childSet) == 0 {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "name or children required")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()

	roots := resolveSearchRoots(req.Hint)
	// Mount points that exist but are empty usually mean a drive was not mounted into the container.
	emptyRoots := []string{}
	for _, r := range roots {
		if entries, err := os.ReadDir(r); err == nil && len(entries) == 0 {
			emptyRoots = append(emptyRoots, r)
		}
	}

	cands, timedOut := searchFoldersByName(ctx, roots, name, childSet)
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].Score != cands[j].Score {
			return cands[i].Score > cands[j].Score
		}
		return len(cands[i].Path) < len(cands[j].Path)
	})
	if len(cands) > 8 {
		cands = cands[:8]
	}

	resolved := len(cands) == 1 || (len(cands) > 1 && cands[0].Score >= 100 && cands[0].Score > cands[1].Score)
	path := ""
	if resolved {
		path = cands[0].Path
	}
	if cands == nil {
		cands = []resolveCandidate{}
	}

	c.JSON(http.StatusOK, gin.H{
		"resolved":   resolved,
		"path":       path,
		"candidates": cands,
		"timed_out":  timedOut,
		"searched":   roots,
		"empty":      emptyRoots,
	})
}
