package filebrowser

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type FileInfo struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
	Mode    string `json:"mode"`
}

const (
	maxTextFileSize              = 2 << 20
	defaultPreviewSize           = 96
	minPreviewSize               = 32
	maxPreviewSize               = 256
	embeddedViewerFrameAncestors = "frame-ancestors 'self' chrome-extension: edge-extension: moz-extension:"
)

type FileBrowser struct{}

func New() *FileBrowser {
	return &FileBrowser{}
}

func (fb *FileBrowser) ListDir(username, path string) ([]FileInfo, error) {
	resolved, err := fb.resolvePath(username, path)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, fmt.Errorf("cannot read directory: %w", err)
	}

	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().UnixMilli(),
			Mode:    info.Mode().String(),
		})
	}

	// Sort: directories first, then alphabetical
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	return files, nil
}

func (fb *FileBrowser) Mkdir(username, path string) error {
	resolved, err := fb.resolvePath(username, path)
	if err != nil {
		return err
	}
	return os.MkdirAll(resolved, 0755)
}

func (fb *FileBrowser) Delete(username, path string) error {
	resolved, err := fb.resolvePath(username, path)
	if err != nil {
		return err
	}
	homeDir, err := fb.getHomeDir(username)
	if err != nil {
		return err
	}
	if resolved == homeDir {
		return errors.New("cannot delete home directory")
	}
	return os.RemoveAll(resolved)
}

func (fb *FileBrowser) Rename(username, oldPath, newPath string) error {
	resolvedOld, err := fb.resolvePath(username, oldPath)
	if err != nil {
		return err
	}
	resolvedNew, err := fb.resolvePath(username, newPath)
	if err != nil {
		return err
	}
	return os.Rename(resolvedOld, resolvedNew)
}

func (fb *FileBrowser) Copy(username, oldPath, newPath string) error {
	resolvedOld, err := fb.resolvePath(username, oldPath)
	if err != nil {
		return err
	}
	resolvedNew, err := fb.resolvePath(username, newPath)
	if err != nil {
		return err
	}
	if resolvedOld == resolvedNew {
		return errors.New("source and destination are the same")
	}

	info, err := os.Stat(resolvedOld)
	if err != nil {
		return fmt.Errorf("source not found: %w", err)
	}
	if _, err := os.Stat(resolvedNew); err == nil {
		return errors.New("destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot stat destination: %w", err)
	}
	if info.IsDir() && isInsideDir(resolvedNew, resolvedOld) {
		return errors.New("cannot copy directory into itself")
	}

	return copyPath(resolvedOld, resolvedNew, info)
}

func (fb *FileBrowser) Upload(username, dir, filename string, data []byte) error {
	resolvedDir, err := fb.resolvePath(username, dir)
	if err != nil {
		return err
	}
	// Sanitize filename
	filename = filepath.Base(filename)
	if filename == "." || filename == ".." {
		return errors.New("invalid filename")
	}
	dest := filepath.Join(resolvedDir, filename)
	return os.WriteFile(dest, data, 0644)
}

func (fb *FileBrowser) ReadText(username, path string) (string, error) {
	resolved, err := fb.resolvePath(username, path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}
	if info.IsDir() {
		return "", errors.New("cannot open directory in editor")
	}
	if info.Size() > maxTextFileSize {
		return "", fmt.Errorf("file too large for editor (max %d MB)", maxTextFileSize>>20)
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	if !utf8.Valid(data) || bytesContainNUL(data) {
		return "", errors.New("file does not appear to be plain text")
	}
	return string(data), nil
}

func (fb *FileBrowser) WriteText(username, path, content string) error {
	if len(content) > maxTextFileSize {
		return fmt.Errorf("file too large for editor (max %d MB)", maxTextFileSize>>20)
	}

	resolved, err := fb.resolvePath(username, path)
	if err != nil {
		return err
	}

	mode := os.FileMode(0644)
	if info, statErr := os.Stat(resolved); statErr == nil {
		if info.IsDir() {
			return errors.New("cannot save over directory")
		}
		mode = info.Mode().Perm()
	}

	return os.WriteFile(resolved, []byte(content), mode)
}

func (fb *FileBrowser) Download(username, path string, w http.ResponseWriter, r *http.Request) {
	resolved, _, err := fb.resolveRegularFile(username, path)
	if err != nil {
		fb.writeFileError(w, err)
		return
	}
	inline := wantsInline(r)
	if allowsEmbeddedViewerFrame(resolved, inline) {
		// Chromium-family browsers often hand off inline PDFs to an internal extension viewer,
		// which is blocked by SAMEORIGIN even when the request starts from our own page.
		w.Header().Del("X-Frame-Options")
		w.Header().Set("Content-Security-Policy", embeddedViewerFrameAncestors)
	}
	fb.serveResolvedFile(w, r, resolved, inline)
}

func (fb *FileBrowser) Preview(username, path string, size int, w http.ResponseWriter, r *http.Request) {
	resolved, info, err := fb.resolveRegularFile(username, path)
	if err != nil {
		fb.writeFileError(w, err)
		return
	}

	if !supportsPreviewResize(resolved) {
		fb.serveResolvedFile(w, r, resolved, true)
		return
	}

	img, err := decodeImageFile(resolved)
	if err != nil {
		fb.serveResolvedFile(w, r, resolved, true)
		return
	}

	preview := resizeImageToFit(img, clampPreviewSize(size))
	var buf bytes.Buffer
	if err := png.Encode(&buf, preview); err != nil {
		fb.serveResolvedFile(w, r, resolved, true)
		return
	}

	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Type", "image/png")
	http.ServeContent(w, r, filepath.Base(resolved)+".preview.png", info.ModTime(), bytes.NewReader(buf.Bytes()))
}

func wantsInline(r *http.Request) bool {
	inline := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("inline")))
	return inline == "1" || inline == "true" || inline == "yes"
}

func allowsEmbeddedViewerFrame(resolved string, inline bool) bool {
	if !inline {
		return false
	}

	switch strings.ToLower(filepath.Ext(resolved)) {
	case ".pdf", ".pptx":
		return true
	default:
		return false
	}
}

func bytesContainNUL(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func copyPath(src, dest string, info os.FileInfo) error {
	if info.IsDir() {
		return copyDir(src, dest, info.Mode().Perm())
	}
	return copyFile(src, dest, info.Mode().Perm())
}

func copyDir(src, dest string, mode os.FileMode) error {
	if err := os.Mkdir(dest, mode); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if err := copyPath(filepath.Join(src, entry.Name()), filepath.Join(dest, entry.Name()), entryInfo); err != nil {
			return err
		}
	}

	return os.Chmod(dest, mode)
}

func copyFile(src, dest string, mode os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return err
	}

	return destFile.Chmod(mode)
}

func sanitizeFilenameASCII(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r >= 0x20 && r < 0x7f && r != '"' && r != '\\' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	result := b.String()
	if result == "" {
		return "download"
	}
	return result
}

func (fb *FileBrowser) resolveRegularFile(username, path string) (string, os.FileInfo, error) {
	resolved, err := fb.resolvePath(username, path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil, os.ErrNotExist
		}
		return "", nil, err
	}
	if info.IsDir() {
		return "", nil, errors.New("cannot open directory")
	}
	return resolved, info, nil
}

func (fb *FileBrowser) serveResolvedFile(w http.ResponseWriter, r *http.Request, resolved string, inline bool) {
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	filename := filepath.Base(resolved)
	asciiName := sanitizeFilenameASCII(filename)
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`,
			disposition, asciiName, url.PathEscape(filename)))
	http.ServeFile(w, r, resolved)
}

func (fb *FileBrowser) writeFileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, os.ErrNotExist):
		http.Error(w, "file not found", http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func clampPreviewSize(size int) int {
	switch {
	case size <= 0:
		return defaultPreviewSize
	case size < minPreviewSize:
		return minPreviewSize
	case size > maxPreviewSize:
		return maxPreviewSize
	default:
		return size
	}
}

func supportsPreviewResize(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif":
		return true
	default:
		return false
	}
}

func decodeImageFile(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	return img, err
}

func resizeImageToFit(src image.Image, maxEdge int) *image.RGBA {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}

	dstW, dstH := fitWithin(srcW, srcH, maxEdge)
	if dstW <= 0 {
		dstW = 1
	}
	if dstH <= 0 {
		dstH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		srcY := srcBounds.Min.Y + (y*srcH)/dstH
		for x := 0; x < dstW; x++ {
			srcX := srcBounds.Min.X + (x*srcW)/dstW
			dst.Set(x, y, color.RGBAModel.Convert(src.At(srcX, srcY)))
		}
	}
	return dst
}

func fitWithin(width, height, maxEdge int) (int, int) {
	if width <= maxEdge && height <= maxEdge {
		return width, height
	}
	if width >= height {
		return maxEdge, max(1, (height*maxEdge)/width)
	}
	return max(1, (width*maxEdge)/height), maxEdge
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// resolvePath resolves a user-provided path to an absolute path,
// ensuring it stays within the user's home directory.
func (fb *FileBrowser) resolvePath(username, path string) (string, error) {
	homeDir, err := fb.getHomeDir(username)
	if err != nil {
		return "", err
	}

	// Handle ~ and relative paths
	if path == "" || path == "~" {
		return homeDir, nil
	}
	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(homeDir, path[2:])
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(homeDir, path)
	}

	// Clean the path
	path = filepath.Clean(path)

	// Resolve symlinks
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		// Path might not exist yet (for mkdir), check parent
		parent := filepath.Dir(path)
		resolvedParent, err2 := filepath.EvalSymlinks(parent)
		if err2 != nil {
			return "", fmt.Errorf("invalid path: %w", err)
		}
		if !isInsideDir(resolvedParent, homeDir) {
			return "", errors.New("access denied: path outside home directory")
		}
		return path, nil
	}

	// Ensure resolved path is within home directory
	if !isInsideDir(resolved, homeDir) {
		return "", errors.New("access denied: path outside home directory")
	}

	return resolved, nil
}

// isInsideDir checks if path is inside dir using proper path-boundary check.
// Prevents /home/ian2 from matching /home/ian.
func isInsideDir(path, dir string) bool {
	if path == dir {
		return true
	}
	// Ensure dir ends with separator for prefix check
	prefix := dir
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(path, prefix)
}

func (fb *FileBrowser) getHomeDir(username string) (string, error) {
	if u, err := user.Lookup(username); err == nil && u.HomeDir != "" {
		return u.HomeDir, nil
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home, nil
	}
	return "", fmt.Errorf("cannot determine home directory for user %q", username)
}
