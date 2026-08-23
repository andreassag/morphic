package shared

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OpenNativeFolderDialog opens a native folder selection dialog.
// Returns the selected folder path, whether a dialog tool is available, and any error.
func OpenNativeFolderDialog(ctx context.Context) (string, bool, error) {
	// Test mode: allow test to set folder via environment
	if testFolder := os.Getenv("MORPHIC_TEST_FOLDER"); testFolder != "" {
		return testFolder, true, nil
	}

	switch runtime.GOOS {
	case "linux":
		return linuxFolderDialog(ctx)
	case "darwin":
		return macFolderDialog(ctx)
	case "windows":
		return windowsFolderDialog(ctx)
	default:
		return "", false, nil
	}
}

func isWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if b, err := os.ReadFile("/proc/version"); err == nil {
		content := strings.ToLower(string(b))
		if strings.Contains(content, "microsoft") || strings.Contains(content, "wsl") {
			return true
		}
	}
	if _, err := os.Stat("/proc/sys/fs/binfmt_misc/WSLInterop"); err == nil {
		return true
	}
	return false
}

func linuxFolderDialog(ctx context.Context) (string, bool, error) {
	// If running under WSL, prefer PowerShell Windows dialog
	if isWSL() {
		if path, err := exec.LookPath("powershell.exe"); err == nil && path != "" {
			script := `Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.FolderBrowserDialog; if ($f.ShowDialog() -eq 'OK') { $f.SelectedPath }`
			cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
			out, err := cmd.Output()
			if err != nil {
				// Cancelled or exited non-zero
				return "", true, nil
			}
			winPath := strings.TrimSpace(string(out))
			if winPath == "" {
				return "", true, nil // user cancelled
			}
			return windowsPathToWSL(ctx, winPath), true, nil
		}
	}

	// Try zenity first
	if path, err := exec.LookPath("zenity"); err == nil && path != "" {
		cmd := exec.CommandContext(ctx, "zenity", "--file-selection", "--directory", "--title=Select Folder")
		out, err := cmd.Output()
		if err == nil {
			return strings.TrimSpace(string(out)), true, nil
		}
		return "", true, nil // tool available but user cancelled
	}

	// Try kdialog
	if path, err := exec.LookPath("kdialog"); err == nil && path != "" {
		cmd := exec.CommandContext(ctx, "kdialog", "--getexistingdirectory", ".")
		out, err := cmd.Output()
		if err == nil {
			return strings.TrimSpace(string(out)), true, nil
		}
		return "", true, nil // tool available but user cancelled
	}

	// No dialog tool found
	return "", false, nil
}

func windowsPathToWSL(ctx context.Context, winPath string) string {
	if out, err := exec.CommandContext(ctx, "wslpath", "-u", winPath).Output(); err == nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed != "" {
			return trimmed
		}
	}

	// Fallback conversion for C:\foo\bar -> /mnt/c/foo/bar
	if len(winPath) >= 3 && winPath[1] == ':' && (winPath[2] == '\\' || winPath[2] == '/') {
		drive := strings.ToLower(string(winPath[0]))
		rest := strings.ReplaceAll(winPath[3:], "\\", "/")
		return "/mnt/" + drive + "/" + rest
	}
	return winPath
}

func macFolderDialog(ctx context.Context) (string, bool, error) {
	cmd := exec.CommandContext(ctx, "osascript", "-e", `POSIX path of (choose folder with prompt "Select Folder")`)
	out, err := cmd.Output()
	if err != nil {
		return "", true, nil
	}
	return strings.TrimSpace(string(out)), true, nil
}

func windowsFolderDialog(ctx context.Context) (string, bool, error) {
	script := `Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.FolderBrowserDialog; if ($f.ShowDialog() -eq 'OK') { $f.SelectedPath }`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return "", true, nil
	}
	return strings.TrimSpace(string(out)), true, nil
}
