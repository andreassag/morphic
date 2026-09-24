package shared

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSafePath(t *testing.T) {
	t.Run("rejects empty path", func(t *testing.T) {
		_, err := ValidateSafePath("")
		assert.ErrorIs(t, err, ErrEmptyPath)
	})

	t.Run("rejects null byte in path", func(t *testing.T) {
		_, err := ValidateSafePath("/tmp/foo\x00bar.jpg")
		assert.ErrorIs(t, err, ErrNullByte)
	})

	t.Run("rejects root directory /", func(t *testing.T) {
		_, err := ValidateSafePath("/")
		assert.ErrorIs(t, err, ErrProtectedSystem)
	})

	t.Run("rejects protected unix system directories", func(t *testing.T) {
		targets := []string{"/etc", "/etc/passwd", "/dev/sda", "/proc/cpuinfo", "/sys/class"}
		for _, target := range targets {
			_, err := ValidateSafePath(target)
			assert.ErrorIs(t, err, ErrProtectedSystem, "target: %s", target)
		}
	})

	t.Run("accepts temporary test directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		safePath, err := ValidateSafePath(tmpDir)
		require.NoError(t, err)
		assert.Equal(t, tmpDir, safePath)
	})
}

func TestValidateSafeDirPath(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid directory", func(t *testing.T) {
		p, err := ValidateSafeDirPath(tmpDir)
		require.NoError(t, err)
		assert.Equal(t, tmpDir, p)
	})

	t.Run("non-existent directory", func(t *testing.T) {
		_, err := ValidateSafeDirPath(filepath.Join(tmpDir, "nonexistent"))
		assert.Error(t, err)
	})

	t.Run("file instead of directory", func(t *testing.T) {
		fPath := filepath.Join(tmpDir, "file.txt")
		require.NoError(t, os.WriteFile(fPath, []byte("hello"), 0o600))
		_, err := ValidateSafeDirPath(fPath)
		assert.ErrorIs(t, err, ErrNotADirectory)
	})
}

func TestValidateMediaFilePath(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid image file", func(t *testing.T) {
		imgPath := filepath.Join(tmpDir, "sample.jpg")
		require.NoError(t, os.WriteFile(imgPath, []byte("jpgdata"), 0o600))

		p, err := ValidateMediaFilePath(imgPath)
		require.NoError(t, err)
		assert.Equal(t, imgPath, p)
	})

	t.Run("unsupported media extension", func(t *testing.T) {
		txtPath := filepath.Join(tmpDir, "sample.txt")
		require.NoError(t, os.WriteFile(txtPath, []byte("txtdata"), 0o600))

		_, err := ValidateMediaFilePath(txtPath)
		assert.ErrorIs(t, err, ErrUnsupportedMedia)
	})

	t.Run("directory matching image name", func(t *testing.T) {
		dirPath := filepath.Join(tmpDir, "fake.png")
		require.NoError(t, os.Mkdir(dirPath, 0o750))

		_, err := ValidateMediaFilePath(dirPath)
		assert.ErrorIs(t, err, ErrNotARegularFile)
	})
}
