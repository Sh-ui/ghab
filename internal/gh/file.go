package gh

import (
	"bytes"
	"fmt"
	"net/url"
)

// maxFileSize is the fetch cap from BUILD.md's M2 spec: a file bigger than
// this is refused before any network request is made.
const maxFileSize = 1 << 20 // 1 MB

// sniffWindow is how much of a file's head is checked for a null byte to
// classify it as binary.
const sniffWindow = 8 << 10 // 8 KB

// TooLargeError marks a file whose reported size exceeds maxFileSize; the
// blob is refused before any network request, per BUILD.md's M2 spec.
type TooLargeError struct {
	Path string
	Size int64
}

func (e *TooLargeError) Error() string {
	return fmt.Sprintf("%s: too large to preview (%s)", e.Path, HumanBytes(e.Size))
}

// BinaryError marks a file whose content sniffs as binary (a null byte in
// the first sniffWindow bytes).
type BinaryError struct {
	Path string
	Size int64
}

func (e *BinaryError) Error() string {
	return fmt.Sprintf("%s: binary file (%s)", e.Path, HumanBytes(e.Size))
}

// FileRaw fetches a file's raw content from
// repos/{owner}/{repo}/contents/{path}?ref={ref}. size is the tree entry's
// reported size, checked BEFORE any request is made -- a file over 1 MB
// never touches the network. The response is also sniffed for binary
// content (a null byte in the first 8 KB); a binary hit returns a
// BinaryError carrying the fetched byte count instead of the bytes.
func (c *Client) FileRaw(owner, repo, path, ref string, size int64) ([]byte, error) {
	if size > maxFileSize {
		return nil, &TooLargeError{Path: path, Size: size}
	}

	reqPath := fmt.Sprintf("repos/%s/%s/contents/%s?ref=%s", owner, repo, escapePath(path), url.QueryEscape(ref))
	if cached, ok := c.cache.get(reqPath); ok {
		if b, ok := cached.([]byte); ok {
			return b, nil
		}
	}

	data, err := c.getRaw(reqPath)
	if err != nil {
		return nil, err
	}

	window := data
	if len(window) > sniffWindow {
		window = window[:sniffWindow]
	}
	if bytes.IndexByte(window, 0) >= 0 {
		return nil, &BinaryError{Path: path, Size: int64(len(data))}
	}

	c.cache.set(reqPath, data)
	return data, nil
}

// HumanBytes formats a byte count the way ghab's UI shows it in placeholder
// lines ("binary file (243 KB)", "file too large (2.1 MB)"): whole KB below
// 1 MB, one decimal MB at or above it.
func HumanBytes(n int64) string {
	const kb = 1024
	switch {
	case n >= kb*kb:
		return fmt.Sprintf("%.1f MB", float64(n)/(kb*kb))
	case n >= kb:
		return fmt.Sprintf("%d KB", n/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
