package gh

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DownloadAsset streams a release asset from url (an Asset's URL field --
// the API endpoint, not BrowserDownloadURL, so this works against private
// repos too) to destPath, creating destPath's parent directories as
// needed. progress is called after every chunk read with the bytes copied
// so far and the total reported via resp.ContentLength; total is 0 when
// the server doesn't report a length, in which case callers should fall
// back to the Asset.Size they already hold (from the Releases fetch) for
// display.
func (c *Client) DownloadAsset(url, destPath string, progress func(done, total int64)) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}

	resp, err := c.assets.Request(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	total := resp.ContentLength
	if total < 0 {
		total = 0 // unknown; callers fall back to the Asset.Size they already hold
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var done int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return nil
}

// ExpandPath expands a leading "~" (or "~/...") to the user's home
// directory, e.g. for [behavior].clone_dir ("~/Developer", "~/src").
// Paths without a leading "~" pass through unchanged.
func ExpandPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// AssetDestPath resolves the local path a downloaded release asset writes
// to: cloneDir (expanded) + "/downloads/" + assetName, per BUILD.md's M3
// spec. The asset name is base-named so a hostile name with path
// separators can't write outside the downloads dir.
func AssetDestPath(cloneDir, assetName string) string {
	return filepath.Join(ExpandPath(cloneDir), "downloads", filepath.Base(filepath.FromSlash(assetName)))
}
