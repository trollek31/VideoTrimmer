//go:build windows

package main

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const appName = "Video Trimmer"

const (
	DT_CENTER       = 0x00000001
	DT_VCENTER      = 0x00000004
	DT_SINGLELINE   = 0x00000020
	DT_END_ELLIPSIS = 0x00008000

	WS_OVERLAPPED    = 0x00000000
	WS_CAPTION       = 0x00C00000
	WS_SYSMENU       = 0x00080000
	WS_THICKFRAME    = 0x00040000
	WS_MINIMIZEBOX   = 0x00020000
	WS_MAXIMIZEBOX   = 0x00010000
	WS_VISIBLE       = 0x10000000
	WS_CHILD         = 0x40000000
	WS_CLIPSIBLINGS  = 0x04000000
	WS_CLIPCHILDREN  = 0x02000000
	WS_BORDER        = 0x00800000
	WS_EX_CLIENTEDGE = 0x00000200

	CS_HREDRAW = 0x0002
	CS_VREDRAW = 0x0001

	SW_SHOW = 5
	SW_HIDE = 0

	WM_DESTROY      = 0x0002
	WM_SIZE         = 0x0005
	WM_SETFOCUS     = 0x0007
	WM_KILLFOCUS    = 0x0008
	WM_ERASEBKGND   = 0x0014
	WM_PAINT        = 0x000F
	WM_CLOSE        = 0x0010
	WM_SETCURSOR    = 0x0020
	WM_MOUSEMOVE    = 0x0200
	WM_LBUTTONDOWN  = 0x0201
	WM_LBUTTONUP    = 0x0202
	WM_TIMER        = 0x0113
	WM_COMMAND      = 0x0111
	WM_DROPFILES    = 0x0233
	WM_CTLCOLOREDIT = 0x0133
	WM_KEYDOWN      = 0x0100
	WM_APP          = 0x8000
	WM_SETFONT      = 0x0030

	EN_KILLFOCUS = 0x0200

	SWP_NOSIZE       = 0x0001
	SWP_NOMOVE       = 0x0002
	SWP_NOZORDER     = 0x0004
	SWP_NOACTIVATE   = 0x0010
	SWP_FRAMECHANGED = 0x0020
	SWP_SHOWWINDOW   = 0x0040

	GWLP_STYLE      = -16
	WS_CHILD_WINDOW = 0x40000000

	OFN_READONLY        = 0x00000001
	OFN_OVERWRITEPROMPT = 0x00000002
	OFN_HIDEREADONLY    = 0x00000004
	OFN_NOCHANGEDIR     = 0x00000008
	OFN_EXPLORER        = 0x00080000
	OFN_FILEMUSTEXIST   = 0x00001000
	OFN_PATHMUSTEXIST   = 0x00000800

	TIMER_ID = 1
	TIMER_MS = 120

	IDC_ARROW = 32512
	IDC_HAND  = 32649

	FW_NORMAL           = 400
	FW_MEDIUM           = 500
	FW_SEMIBOLD         = 600
	FW_BOLD             = 700
	DEFAULT_CHARSET     = 1
	OUT_TT_PRECIS       = 4
	CLIP_DEFAULT_PRECIS = 0
	CLEARTYPE_QUALITY   = 5
	DEFAULT_PITCH       = 0

	DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = ^uintptr(3) // (HANDLE)-4

	DWMWA_USE_IMMERSIVE_DARK_MODE  = 20
	DWMWA_WINDOW_CORNER_PREFERENCE = 33
	DWMWCP_ROUND                   = 2

	MB_OK              = 0x00000000
	MB_ICONERROR       = 0x00000010
	MB_ICONWARNING     = 0x00000030
	MB_ICONINFORMATION = 0x00000040

	SW_SHOWNORMAL = 1
	VK_SPACE      = 0x20
	VK_O          = 0x4F
	VK_S          = 0x53
	VK_CONTROL    = 0x11
)

// A small, hand-drawn design system. RGB values are written in Win32's BGR COLORREF order.
const (
	BG       = 0x000B1016
	SURFACE  = 0x00131A23
	SURFACE2 = 0x0018202B
	SURFACE3 = 0x00202A36
	BORDER   = 0x002A3745
	TEXT     = 0x00EFF4FA
	MUTED    = 0x0098A6B5
	ACCENT   = 0x0069E6A8
	ACCENT2  = 0x0058B9FF
	WHITE    = 0x00FFFFFF
	BLACK    = 0x00000000
	RED      = 0x004C596A
	ORANGE   = 0x0052A6FF
	EDIT_BG  = 0x00141C25
)

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }

type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type OPENFILENAMEW struct {
	LStructSize       uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
	LReserved         uintptr
	DwReserved        uint32
	FlagsEx           uint32
}

type Meta struct {
	Streams []struct {
		CodecName  string `json:"codec_name"`
		CodecType  string `json:"codec_type"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		RFrameRate string `json:"r_frame_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		Size     string `json:"size"`
	} `json:"format"`
}

type RectF struct{ x, y, w, h float64 }

const (
	BTN_OPEN        = 1
	BTN_PLAY        = 2
	BTN_SAVE        = 3
	BTN_RESET       = 4
	BTN_MODE_COPY   = 5
	BTN_MODE_EXACT  = 6
	BTN_OPEN_FOLDER = 7
)

type AppState struct {
	mu        sync.Mutex
	main      uintptr
	preview   uintptr
	timeline  uintptr
	startEdit uintptr
	endEdit   uintptr
	editBrush uintptr

	duration      float64
	start         float64
	end           float64
	current       float64
	playing       bool
	exactMode     bool
	currentFile   string
	metaLine      string
	lastOutput    string
	status        string
	progress      int
	exportRunning bool

	player       *exec.Cmd
	playerWindow uintptr
	playerMu     sync.Mutex
	exportCmd    *exec.Cmd
	exportMu     sync.Mutex

	hover        int
	pressed      int
	fontTitle    uintptr
	fontSubtitle uintptr
	fontBody     uintptr
	fontSmall    uintptr
	fontButton   uintptr
	fontMono     uintptr
}

var state = &AppState{}

var runtimeToolsDir string

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
)

var (
	procRegisterClassEx              = user32.NewProc("RegisterClassExW")
	procCreateWindowEx               = user32.NewProc("CreateWindowExW")
	procDefWindowProc                = user32.NewProc("DefWindowProcW")
	procShowWindow                   = user32.NewProc("ShowWindow")
	procUpdateWindow                 = user32.NewProc("UpdateWindow")
	procGetMessage                   = user32.NewProc("GetMessageW")
	procTranslateMessage             = user32.NewProc("TranslateMessage")
	procDispatchMessage              = user32.NewProc("DispatchMessageW")
	procDestroyWindow                = user32.NewProc("DestroyWindow")
	procPostQuit                     = user32.NewProc("PostQuitMessage")
	procGetClientRect                = user32.NewProc("GetClientRect")
	procMoveWindow                   = user32.NewProc("MoveWindow")
	procSetWindowText                = user32.NewProc("SetWindowTextW")
	procGetWindowText                = user32.NewProc("GetWindowTextW")
	procSendMessage                  = user32.NewProc("SendMessageW")
	procPostMessage                  = user32.NewProc("PostMessageW")
	procEnableWindow                 = user32.NewProc("EnableWindow")
	procSetTimer                     = user32.NewProc("SetTimer")
	procKillTimer                    = user32.NewProc("KillTimer")
	procSetFocus                     = user32.NewProc("SetFocus")
	procGetFocus                     = user32.NewProc("GetFocus")
	procInvalidateRect               = user32.NewProc("InvalidateRect")
	procBeginPaint                   = user32.NewProc("BeginPaint")
	procEndPaint                     = user32.NewProc("EndPaint")
	procFillRect                     = user32.NewProc("FillRect")
	procCreateSolidBrush             = gdi32.NewProc("CreateSolidBrush")
	procCreatePen                    = gdi32.NewProc("CreatePen")
	procSelectObject                 = gdi32.NewProc("SelectObject")
	procDeleteObject                 = gdi32.NewProc("DeleteObject")
	procRectangle                    = gdi32.NewProc("Rectangle")
	procRoundRect                    = gdi32.NewProc("RoundRect")
	procEllipse                      = gdi32.NewProc("Ellipse")
	procSetBkColor                   = gdi32.NewProc("SetBkColor")
	procSetBkMode                    = gdi32.NewProc("SetBkMode")
	procSetTextColor                 = gdi32.NewProc("SetTextColor")
	procTextOut                      = gdi32.NewProc("TextOutW")
	procDrawText                     = user32.NewProc("DrawTextW")
	procCreateFont                   = gdi32.NewProc("CreateFontW")
	procGetStockObject               = gdi32.NewProc("GetStockObject")
	procLoadCursor                   = user32.NewProc("LoadCursorW")
	procChooseOpen                   = comdlg32.NewProc("GetOpenFileNameW")
	procChooseSave                   = comdlg32.NewProc("GetSaveFileNameW")
	procMessageBox                   = user32.NewProc("MessageBoxW")
	procTrackMouseEvent              = user32.NewProc("TrackMouseEvent")
	procSetCapture                   = user32.NewProc("SetCapture")
	procReleaseCapture               = user32.NewProc("ReleaseCapture")
	procSetCursor                    = user32.NewProc("SetCursor")
	procGetKeyState                  = user32.NewProc("GetKeyState")
	procSetParent                    = user32.NewProc("SetParent")
	procSetWindowLongPtr             = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos                 = user32.NewProc("SetWindowPos")
	procFindWindow                   = user32.NewProc("FindWindowW")
	procIsWindow                     = user32.NewProc("IsWindow")
	procDragAcceptFiles              = shell32.NewProc("DragAcceptFiles")
	procDragQueryFile                = shell32.NewProc("DragQueryFileW")
	procDragFinish                   = shell32.NewProc("DragFinish")
	procShellExecute                 = shell32.NewProc("ShellExecuteW")
	procDwmSetWindowAttribute        = dwmapi.NewProc("DwmSetWindowAttribute")
	procSetThreadDpiAwarenessContext = user32.NewProc("SetThreadDpiAwarenessContext")
	procGetModuleHandle              = kernel32.NewProc("GetModuleHandleW")
)

var (
	pendingMu       sync.Mutex
	pendingStatus   string
	pendingProgress int
)

func ensureTools() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	base := filepath.Dir(exe)

	// Be tolerant of all supported portable layouts. This is deliberately based on
	// the EXE location, never on the process working directory.
	for _, direct := range []string{
		filepath.Join(base, "bin"),
		filepath.Join(base, "runtime", "bin"),
	} {
		if toolsComplete(direct) {
			runtimeToolsDir = direct
			return nil
		}
	}

	// Also support FFmpeg archives unpacked one folder deeper, e.g.
	// runtime\ffmpeg-...\bin.
	for _, root := range []string{filepath.Join(base, "runtime"), base} {
		if found := findToolsDir(root, 2); found != "" {
			runtimeToolsDir = found
			return nil
		}
	}

	// Fall back to the packaged runtime archive. Different builds may use a
	// slightly different archive filename, so discover the first matching ZIP.
	runtimeDir := filepath.Join(base, "runtime")
	runtimeZip := filepath.Join(runtimeDir, "ffmpeg-runtime.zip")
	if _, statErr := os.Stat(runtimeZip); statErr != nil {
		entries, readErr := os.ReadDir(runtimeDir)
		if readErr == nil {
			for _, entry := range entries {
				name := strings.ToLower(entry.Name())
				if !entry.IsDir() && strings.HasSuffix(name, ".zip") && strings.Contains(name, "ffmpeg") {
					runtimeZip = filepath.Join(runtimeDir, entry.Name())
					break
				}
			}
		}
	}
	if _, err := os.Stat(runtimeZip); err != nil {
		return fmt.Errorf("FFmpeg runtime not found next to the application (expected runtime\\bin or runtime\\ffmpeg-runtime.zip)")
	}

	runtimeInfo, err := os.Stat(runtimeZip)
	if err != nil {
		return err
	}
	cacheRoot := filepath.Join(os.TempDir(), fmt.Sprintf("VideoTrimmerRuntimeV4_%d_%d", runtimeInfo.Size(), runtimeInfo.ModTime().UnixNano()))
	cache := filepath.Join(cacheRoot, "bin")
	if !toolsComplete(cache) {
		if err := os.RemoveAll(cacheRoot); err != nil {
			return err
		}
		if err := extractRuntimeBin(runtimeZip, cache); err != nil {
			return fmt.Errorf("failed to unpack FFmpeg runtime: %w", err)
		}
	}
	if !toolsComplete(cache) {
		return fmt.Errorf("FFmpeg runtime is incomplete: ffmpeg.exe, ffprobe.exe and ffplay.exe are required")
	}
	runtimeToolsDir = cache
	return nil
}

func findToolsDir(root string, maxDepth int) string {
	root = filepath.Clean(root)
	if toolsComplete(root) {
		return root
	}
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if found != "" || walkErr != nil {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		depth := 0
		if rel != "." {
			depth = strings.Count(rel, string(os.PathSeparator)) + 1
		}
		if depth > maxDepth+1 {
			return filepath.SkipDir
		}
		if d.IsDir() && toolsComplete(path) {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

func toolsComplete(dir string) bool {
	for _, name := range []string{"ffmpeg.exe", "ffprobe.exe", "ffplay.exe"} {
		p := filepath.Join(dir, name)
		st, err := os.Stat(p)
		// FFmpeg shared builds can have small .exe files because most of the
		// runtime is stored in DLLs. Presence is the correct validation here.
		if err != nil || st.IsDir() || st.Size() == 0 {
			return false
		}
	}
	return true
}

func extractRuntimeBin(zipPath, outDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		n := strings.ReplaceAll(f.Name, "\\", "/")
		n = strings.TrimPrefix(n, "./")
		lower := strings.ToLower(n)
		idx := strings.Index(lower, "/bin/")
		if idx < 0 {
			if strings.HasPrefix(lower, "bin/") {
				idx = -1
			} else {
				continue
			}
		}
		rel := n
		if idx >= 0 {
			rel = n[idx+5:]
		} else {
			rel = n[len("bin/"):]
		}
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue
		}
		dst := filepath.Join(outDir, filepath.FromSlash(rel))
		relCheck, err := filepath.Rel(outDir, dst)
		if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(os.PathSeparator)) || filepath.IsAbs(relCheck) {
			return fmt.Errorf("invalid runtime entry: %s", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		closeOutErr := out.Close()
		closeInErr := rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}
		if closeInErr != nil {
			return closeInErr
		}
	}
	return nil
}

func toolPath(name string) string {
	if runtimeToolsDir != "" {
		return filepath.Join(runtimeToolsDir, name)
	}
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "bin", name)
	}
	return name
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func utf16buf(s string, max int) []uint16 {
	b := make([]uint16, max)
	u := utf16.Encode([]rune(s))
	copy(b, u)
	return b
}

func loword(v uintptr) uint16     { return uint16(v & 0xffff) }
func hiword(v uintptr) uint16     { return uint16((v >> 16) & 0xffff) }
func xFromLParam(v uintptr) int32 { return int32(int16(loword(v))) }
func yFromLParam(v uintptr) int32 { return int32(int16(hiword(v))) }
func rgb(r, g, b byte) uintptr    { return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16) }

func createWindow(class, title string, style, exstyle uint32, x, y, w, h int32, parent, id uintptr, proc uintptr) uintptr {
	inst, _, _ := procGetModuleHandle.Call(0)
	hwnd, _, _ := procCreateWindowEx.Call(
		uintptr(exstyle), uintptr(unsafe.Pointer(utf16Ptr(class))), uintptr(unsafe.Pointer(utf16Ptr(title))), uintptr(style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, id, inst, 0,
	)
	return hwnd
}

func setText(hwnd uintptr, s string) {
	if hwnd == 0 {
		return
	}
	procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(s))))
}

func getText(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	n, _, _ := procGetWindowText.Call(hwnd, 0, 0)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

func message(text, caption string, flags uintptr) {
	procMessageBox.Call(state.main, uintptr(unsafe.Pointer(utf16Ptr(text))), uintptr(unsafe.Pointer(utf16Ptr(caption))), flags)
}

func loadCursor(id uintptr) uintptr {
	h, _, _ := procLoadCursor.Call(0, id)
	return h
}

func createBrush(color uintptr) uintptr {
	b, _, _ := procCreateSolidBrush.Call(color)
	return b
}

func initFonts() {
	state.fontTitle = makeFont(-24, FW_SEMIBOLD, "Segoe UI")
	state.fontSubtitle = makeFont(-13, FW_NORMAL, "Segoe UI")
	state.fontBody = makeFont(-14, FW_NORMAL, "Segoe UI")
	state.fontSmall = makeFont(-12, FW_MEDIUM, "Segoe UI")
	state.fontButton = makeFont(-13, FW_SEMIBOLD, "Segoe UI")
	state.fontMono = makeFont(-13, FW_MEDIUM, "Cascadia Mono")
}

func makeFont(height, weight int32, face string) uintptr {
	return callCreateFont(height, weight, face)
}

func callCreateFont(height, weight int32, face string) uintptr {
	h, _, _ := procCreateFont.Call(
		uintptr(uint32(height)), 0, 0, 0, uintptr(weight), 0, 0, 0,
		uintptr(DEFAULT_CHARSET), uintptr(OUT_TT_PRECIS), uintptr(CLIP_DEFAULT_PRECIS),
		uintptr(CLEARTYPE_QUALITY), uintptr(DEFAULT_PITCH), uintptr(unsafe.Pointer(utf16Ptr(face))),
	)
	return h
}

func registerClass(class string, wndProc uintptr, brushColor uintptr) {
	inst, _, _ := procGetModuleHandle.Call(0)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         CS_HREDRAW | CS_VREDRAW,
		LpfnWndProc:   wndProc,
		HInstance:     inst,
		HCursor:       loadCursor(IDC_ARROW),
		HbrBackground: createBrush(brushColor),
		LpszClassName: utf16Ptr(class),
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
}

func selectFont(hdc, font uintptr) uintptr {
	if font == 0 {
		return 0
	}
	old, _, _ := procSelectObject.Call(hdc, font)
	return old
}

func drawText(hdc uintptr, s string, r RECT, color, f uintptr, flags int32) {
	old := selectFont(hdc, f)
	procSetBkMode.Call(hdc, 1)
	procSetTextColor.Call(hdc, color)
	buf := utf16.Encode([]rune(s + "\x00"))
	allFlags := flags | 0x00000800 // DT_NOPREFIX
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(&buf[0])), ^uintptr(0), uintptr(unsafe.Pointer(&r)), uintptr(allFlags))
	if old != 0 {
		procSelectObject.Call(hdc, old)
	}
}

func fill(hdc uintptr, r RECT, color uintptr) {
	b := createBrush(color)
	if b == 0 {
		return
	}
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), b)
	procDeleteObject.Call(b)
}

func roundRect(hdc uintptr, r RECT, fillColor, borderColor uintptr, radius int32) {
	b := createBrush(fillColor)
	p, _, _ := procCreatePen.Call(0, 1, borderColor)
	oldB, _, _ := procSelectObject.Call(hdc, b)
	oldP, _, _ := procSelectObject.Call(hdc, p)
	procRoundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(radius), uintptr(radius))
	procSelectObject.Call(hdc, oldB)
	procSelectObject.Call(hdc, oldP)
	if b != 0 {
		procDeleteObject.Call(b)
	}
	if p != 0 {
		procDeleteObject.Call(p)
	}
}

func line(hdc uintptr, x1, y1, x2, y2 int32, color uintptr, width int32) {
	p, _, _ := procCreatePen.Call(0, uintptr(width), color)
	old, _, _ := procSelectObject.Call(hdc, p)
	moveToEx.Call(hdc, uintptr(x1), uintptr(y1), 0)
	lineTo.Call(hdc, uintptr(x2), uintptr(y2))
	procSelectObject.Call(hdc, old)
	if p != 0 {
		procDeleteObject.Call(p)
	}
}

var (
	moveToEx = gdi32.NewProc("MoveToEx")
	lineTo   = gdi32.NewProc("LineTo")
)

func makeEdit(parent uintptr, id uintptr, text string) uintptr {
	style := uint32(WS_CHILD|WS_VISIBLE) | 0x0001 | 0x0080 // ES_CENTER + ES_AUTOHSCROLL
	h := createWindow("EDIT", text, style, WS_EX_CLIENTEDGE, 0, 0, 120, 34, parent, id, 0)
	if h != 0 {
		procSendMessage.Call(h, WM_SETFONT, state.fontMono, 1)
	}
	return h
}

func buildUI(hwnd uintptr) {
	state.preview = createWindow("VideoTrimmerPreview", "", WS_CHILD|WS_VISIBLE|WS_CLIPSIBLINGS|WS_CLIPCHILDREN, 0, 0, 0, 0, 0, hwnd, 1001, 0)
	state.timeline = createWindow("VideoTrimmerTimeline", "", WS_CHILD|WS_VISIBLE, 0, 0, 0, 0, 0, hwnd, 1002, 0)
	state.startEdit = makeEdit(hwnd, 2001, "00:00:00.000")
	state.endEdit = makeEdit(hwnd, 2002, "00:00:00.000")
	state.editBrush, _, _ = procCreateSolidBrush.Call(EDIT_BG)
	state.status = "Откройте MP4 или перетащите его сюда"
	state.currentFile = ""
	state.metaLine = ""
	layoutUI(hwnd, 1180, 780)
	procInvalidateRect.Call(hwnd, 0, 1)
}

var (
	rectOpen      RectF
	rectPlay      RectF
	rectSave      RectF
	rectReset     RectF
	rectModeGroup RectF
	rectFolder    RectF
)

func layoutUI(hwnd uintptr, width, height int32) {
	if width < 980 {
		width = 980
	}
	if height < 680 {
		height = 680
	}
	margin := int32(26)
	headerH := int32(72)
	previewY := headerH
	timelineH := int32(96)
	controlsH := int32(88)
	footerH := int32(60)
	gap := int32(14)
	previewH := height - headerH - timelineH - controlsH - footerH - gap*3
	if previewH < 300 {
		previewH = 300
	}
	previewW := width - margin*2
	procMoveWindow.Call(state.preview, uintptr(margin), uintptr(previewY), uintptr(previewW), uintptr(previewH), 1)

	tlY := previewY + previewH + gap
	procMoveWindow.Call(state.timeline, uintptr(margin), uintptr(tlY), uintptr(previewW), uintptr(timelineH), 1)

	controlsY := tlY + timelineH + gap
	fieldW := int32(140)
	fieldH := int32(36)
	procMoveWindow.Call(state.startEdit, uintptr(margin+116), uintptr(controlsY+24), uintptr(fieldW), uintptr(fieldH), 1)
	procMoveWindow.Call(state.endEdit, uintptr(margin+274), uintptr(controlsY+24), uintptr(fieldW), uintptr(fieldH), 1)

	rectOpen = RectF{float64(width - 190), 16, 156, 40}
	rectPlay = RectF{float64(margin), float64(controlsY + 24), 104, 36}
	rectReset = RectF{float64(margin + 228), float64(controlsY + 24), 72, 36}
	rectModeGroup = RectF{float64(margin + 514), float64(controlsY + 8), 220, 58}
	rectSave = RectF{float64(width - 200), float64(controlsY + 18), 174, 46}
	rectFolder = RectF{float64(width - 170), float64(controlsY + 94), 144, 32}
	procInvalidateRect.Call(hwnd, 0, 1)
	resizePlayerWindow()
}

func rectContains(r RectF, x, y int32) bool {
	return float64(x) >= r.x && float64(x) <= r.x+r.w && float64(y) >= r.y && float64(y) <= r.y+r.h
}

func previewWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_PAINT:
		drawPreview(hwnd)
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_SIZE:
		resizePlayerWindow()
	}
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func cyForPreview(r RECT) int32 { return (r.Top + r.Bottom) / 2 }

func drawPreview(hwnd uintptr) {
	var ps PAINTSTRUCT
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var r RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	fill(hdc, r, SURFACE2)

	state.mu.Lock()
	file := state.currentFile
	duration := state.duration
	state.mu.Unlock()
	if file != "" && duration > 0 {
		state.playerMu.Lock()
		ph := state.playerWindow
		state.playerMu.Unlock()
		if ph != 0 {
			return
		}
		state.mu.Lock()
		status := state.status
		state.mu.Unlock()
		label := "Загрузка предпросмотра…"
		if status == "Предпросмотр недоступен" || strings.HasPrefix(status, "Не удалось запустить предпросмотр") {
			label = "Предпросмотр недоступен"
		}
		drawText(hdc, label, RECT{r.Left + 20, cyForPreview(r) - 12, r.Right - 20, cyForPreview(r) + 18}, MUTED, state.fontSubtitle, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		return
	}
	cx := (r.Left + r.Right) / 2
	cy := (r.Top+r.Bottom)/2 - 16
	circle := createBrush(rgb(33, 44, 58))
	procSelectObject.Call(hdc, circle)
	procEllipse.Call(hdc, uintptr(cx-34), uintptr(cy-34), uintptr(cx+34), uintptr(cy+34))
	procDeleteObject.Call(circle)
	drawText(hdc, "▶", RECT{cx - 14, cy - 16, cx + 14, cy + 16}, ACCENT, state.fontTitle, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, "Перетащите MP4 сюда", RECT{r.Left + 20, cy + 54, r.Right - 20, cy + 82}, TEXT, state.fontTitle, DT_CENTER|DT_SINGLELINE)
	drawText(hdc, "или нажмите «Открыть видео» сверху", RECT{r.Left + 20, cy + 88, r.Right - 20, cy + 112}, MUTED, state.fontSubtitle, DT_CENTER|DT_SINGLELINE)
}

func drawMain(hwnd uintptr, hdc uintptr) {
	var r RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	fill(hdc, r, BG)

	// Header
	roundRect(hdc, RECT{18, 14, 64, 60}, SURFACE3, BORDER, 12)
	drawText(hdc, "▶", RECT{18, 21, 64, 54}, ACCENT, state.fontTitle, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, appName, RECT{76, 17, 290, 44}, TEXT, state.fontTitle, DT_SINGLELINE)
	drawText(hdc, "Быстрая обрезка • без лишнего", RECT{76, 44, 330, 66}, MUTED, state.fontSubtitle, DT_SINGLELINE)

	buttonDraw(hdc, rectOpen, BTN_OPEN, "Открыть видео", ACCENT2, true)

	// The preview/timeline are child windows; this layer draws the shell around them.
	state.mu.Lock()
	file := state.currentFile
	progress := state.progress
	status := state.status
	exact := state.exactMode
	running := state.exportRunning
	metaLine := state.metaLine
	out := state.lastOutput
	state.mu.Unlock()
	_ = file

	tdraw := func(s string, rr RECT, color uintptr, f uintptr, flags int32) { drawText(hdc, s, rr, color, f, flags) }

	var prevRect RECT
	procGetClientRect.Call(state.preview, uintptr(unsafe.Pointer(&prevRect)))
	previewH := prevRect.Bottom
	timelineY := int32(78) + previewH + 16
	controlsY := timelineY + 108 + 0 + 0 + 0
	infoY := controlsY

	// Time fields
	tdraw("НАЧАЛО", RECT{142, infoY - 4, 296, infoY + 18}, MUTED, state.fontSmall, 0)
	tdraw("КОНЕЦ", RECT{300, infoY - 4, 454, infoY + 18}, MUTED, state.fontSmall, 0)
	inputCard(hdc, RectF{142, float64(infoY + 20), 154, 36})
	inputCard(hdc, RectF{300, float64(infoY + 20), 140, 36})

	buttonDraw(hdc, rectPlay, BTN_PLAY, "▶  Пуск", ACCENT2, false)
	buttonDraw(hdc, rectReset, BTN_RESET, "Сброс", SURFACE3, false)
	modeGroupDraw(hdc, rectModeGroup, exact)
	buttonDraw(hdc, rectSave, BTN_SAVE, "Сохранить", ACCENT, true)

	// Status bar
	statusY := infoY + 74
	if running {
		status = "Обработка видео…"
	}
	var statusColor uintptr = MUTED
	if strings.HasPrefix(status, "Видео успешно") {
		statusColor = ACCENT
	}
	drawText(hdc, status, RECT{26, statusY, int32(r.Right - 26), statusY + 22}, statusColor, state.fontSmall, 0)
	if file != "" {
		name := filepath.Base(file)
		drawText(hdc, name, RECT{26, statusY + 22, int32(r.Right - 330), statusY + 45}, TEXT, state.fontSmall, 0)
		drawText(hdc, metaLine, RECT{26, statusY + 42, int32(r.Right - 330), statusY + 63}, MUTED, state.fontSmall, 0)
	}
	if out != "" {
		buttonDraw(hdc, rectFolder, BTN_OPEN_FOLDER, "Открыть папку", SURFACE3, false)
	}

	// Export progress line
	if progress > 0 && running {
		pr := RECT{26, r.Bottom - 9, int32(float64(r.Right-26) * float64(progress) / 100.0), r.Bottom - 5}
		fill(hdc, pr, ACCENT)
	}
}

func inputCard(hdc uintptr, r RectF) {
	rr := RECT{int32(r.x), int32(r.y), int32(r.x + r.w), int32(r.y + r.h)}
	roundRect(hdc, rr, EDIT_BG, BORDER, 8)
}

func buttonDraw(hdc uintptr, r RectF, id int, title string, accent uintptr, filled bool) {
	state.mu.Lock()
	disabled := (id == BTN_SAVE || id == BTN_PLAY || id == BTN_RESET || id == BTN_OPEN || id == BTN_MODE_COPY || id == BTN_MODE_EXACT) && (state.currentFile == "" || state.exportRunning)
	if id == BTN_OPEN {
		disabled = state.exportRunning
	}
	if id == BTN_MODE_COPY || id == BTN_MODE_EXACT {
		disabled = state.exportRunning || state.currentFile == ""
	}
	if id == BTN_OPEN_FOLDER && (state.lastOutput == "" || state.exportRunning) {
		disabled = true
	}
	state.mu.Unlock()
	h := state.hover == id && !disabled
	var fillC uintptr = SURFACE3
	var borderC uintptr = BORDER
	var textC uintptr = TEXT
	if disabled {
		fillC, borderC, textC = SURFACE, BORDER, MUTED
	}
	if filled && !disabled {
		fillC = accent
		borderC = accent
		textC = BG
	} else if h {
		fillC = 0x0043362A
	}
	rr := RECT{int32(r.x), int32(r.y), int32(r.x + r.w), int32(r.y + r.h)}
	roundRect(hdc, rr, fillC, borderC, 9)
	flags := int32(0x00000001 | 0x00000100 | 0x00000020)
	drawText(hdc, title, rr, textC, state.fontButton, flags)
}

func modeGroupDraw(hdc uintptr, r RectF, exact bool) {
	rr := RECT{int32(r.x), int32(r.y), int32(r.x + r.w), int32(r.y + r.h)}
	roundRect(hdc, rr, SURFACE, BORDER, 10)
	mid := rr.Left + (rr.Right-rr.Left)/2
	var leftFill, rightFill uintptr = SURFACE, SURFACE
	var leftBorder, rightBorder uintptr = BORDER, BORDER
	if !exact {
		leftFill, leftBorder = 0x00252115, ACCENT
	} else {
		rightFill, rightBorder = 0x002B211A, ORANGE
	}
	if state.hover == BTN_MODE_COPY {
		leftBorder = ACCENT
	}
	if state.hover == BTN_MODE_EXACT {
		rightBorder = ORANGE
	}
	roundRect(hdc, RECT{rr.Left + 2, rr.Top + 2, mid, rr.Bottom - 2}, leftFill, leftBorder, 8)
	roundRect(hdc, RECT{mid, rr.Top + 2, rr.Right - 2, rr.Bottom - 2}, rightFill, rightBorder, 8)
	drawText(hdc, "Без перекодирования", RECT{rr.Left + 7, rr.Top + 7, mid - 6, rr.Top + 28}, TEXT, state.fontSmall, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, "Быстро · исходное качество", RECT{rr.Left + 7, rr.Top + 30, mid - 6, rr.Bottom - 5}, MUTED, state.fontSmall, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, "Точная обрезка", RECT{mid + 6, rr.Top + 7, rr.Right - 7, rr.Top + 28}, TEXT, state.fontSmall, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, "По кадру · перекодирование", RECT{mid + 6, rr.Top + 30, rr.Right - 7, rr.Bottom - 5}, MUTED, state.fontSmall, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}

func metaLineFromMeta(m Meta) string {
	var vcodec, acodec string
	w, h := 0, 0
	fps := 0.0
	for _, s := range m.Streams {
		if s.CodecType == "video" && vcodec == "" {
			vcodec = prettyCodec(s.CodecName)
			w, h = s.Width, s.Height
			fps = parseFPS(s.RFrameRate)
		}
		if s.CodecType == "audio" && acodec == "" {
			acodec = prettyCodec(s.CodecName)
		}
	}
	if vcodec == "" {
		vcodec = "Видео"
	}
	parts := []string{}
	if w > 0 && h > 0 {
		parts = append(parts, fmt.Sprintf("%dx%d", w, h))
	}
	if fps > 0 {
		parts = append(parts, fmt.Sprintf("%.2g FPS", fps))
	}
	parts = append(parts, vcodec)
	if acodec != "" {
		parts = append(parts, acodec)
	}
	return strings.Join(parts, "  ·  ")
}

func videoMetaLine(file string) string {
	m, err := probeVideo(file)
	if err != nil {
		return "MP4"
	}
	var vcodec, acodec string
	w, h := 0, 0
	fps := 0.0
	for _, s := range m.Streams {
		if s.CodecType == "video" && vcodec == "" {
			vcodec = prettyCodec(s.CodecName)
			w, h = s.Width, s.Height
			fps = parseFPS(s.RFrameRate)
		}
		if s.CodecType == "audio" && acodec == "" {
			acodec = prettyCodec(s.CodecName)
		}
	}
	if vcodec == "" {
		vcodec = "Видео"
	}
	parts := []string{}
	if w > 0 && h > 0 {
		parts = append(parts, fmt.Sprintf("%dx%d", w, h))
	}
	if fps > 0 {
		parts = append(parts, fmt.Sprintf("%.2g FPS", fps))
	}
	parts = append(parts, vcodec)
	if acodec != "" {
		parts = append(parts, acodec)
	}
	return strings.Join(parts, "  ·  ")
}

func prettyCodec(s string) string {
	switch strings.ToLower(s) {
	case "h264":
		return "H.264"
	case "hevc":
		return "H.265"
	case "vp9":
		return "VP9"
	case "av1":
		return "AV1"
	case "aac":
		return "AAC"
	case "opus":
		return "Opus"
	default:
		if s == "" {
			return ""
		}
		return strings.ToUpper(s[:1]) + s[1:]
	}
}

func initDWM(hwnd uintptr) {
	mode := uint32(1)
	procDwmSetWindowAttribute.Call(hwnd, DWMWA_USE_IMMERSIVE_DARK_MODE, uintptr(unsafe.Pointer(&mode)), unsafe.Sizeof(mode))
	corners := uint32(DWMWCP_ROUND)
	procDwmSetWindowAttribute.Call(hwnd, DWMWA_WINDOW_CORNER_PREFERENCE, uintptr(unsafe.Pointer(&corners)), unsafe.Sizeof(corners))
}

func main() {
	procSetThreadDpiAwarenessContext.Call(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2)
	if err := ensureTools(); err != nil {
		message("Не найден встроенный FFmpeg.\n\nПрограмма ищет FFmpeg рядом с VideoTrimmer.exe в: bin\\, runtime\\bin\\, вложенной папке runtime\\*\\bin\\ или runtime\\ffmpeg-runtime.zip.\n\nУбедитесь, что portable-архив распакован целиком.", "Неполный portable-дистрибутив", MB_ICONERROR|MB_OK)
		return
	}
	initFonts()
	registerClass("VideoTrimmerMain", syscall.NewCallback(mainWndProc), BG)
	registerClass("VideoTrimmerTimeline", syscall.NewCallback(timelineWndProc), SURFACE)
	registerClass("VideoTrimmerPreview", syscall.NewCallback(previewWndProc), SURFACE2)

	hwnd := createWindow("VideoTrimmerMain", appName, WS_OVERLAPPED|WS_CAPTION|WS_SYSMENU|WS_THICKFRAME|WS_MINIMIZEBOX|WS_MAXIMIZEBOX|WS_CLIPCHILDREN, 0, 120, 80, 1180, 780, 0, 0, 0)
	state.main = hwnd
	initDWM(hwnd)
	buildUI(hwnd)
	procDragAcceptFiles.Call(hwnd, 1)
	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)
	procSetTimer.Call(hwnd, TIMER_ID, TIMER_MS, 0)

	if len(os.Args) > 1 {
		if info, err := os.Stat(os.Args[1]); err == nil && !info.IsDir() {
			go func(p string) {
				time.Sleep(250 * time.Millisecond)
				openVideoPath(p)
			}(os.Args[1])
		}
	}

	var msg struct {
		Hwnd           uintptr
		Message        uint32
		WParam, LParam uintptr
		Time           uint32
		Pt             POINT
	}
	for {
		r, _, err := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) == 0 || (r == ^uintptr(0) && err != nil) {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func mainWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		drawMain(hwnd, hdc)
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_SIZE:
		w := int32(int16(loword(lParam)))
		h := int32(int16(hiword(lParam)))
		if w < 980 || h < 680 {
			procSetWindowPos.Call(hwnd, 0, 0, 0, 1000, 720, SWP_NOMOVE|SWP_NOZORDER|SWP_NOACTIVATE)
			return 0
		}
		layoutUI(hwnd, w, h)
		return 0
	case WM_MOUSEMOVE:
		updateHover(xFromLParam(lParam), yFromLParam(lParam))
		return 0
	case WM_SETCURSOR:
		if state.hover != 0 {
			procSetCursor.Call(loadCursor(IDC_HAND))
			return 1
		}
	case WM_LBUTTONDOWN:
		handleMainClick(xFromLParam(lParam), yFromLParam(lParam))
		return 0
	case WM_LBUTTONUP:
		state.pressed = 0
		procReleaseCapture.Call()
		procInvalidateRect.Call(hwnd, 0, 0)
		return 0
	case WM_KEYDOWN:
		ctrl, _, _ := procGetKeyState.Call(VK_CONTROL)
		switch wParam {
		case VK_SPACE:
			togglePlayback()
			return 0
		case VK_O:
			if ctrl&0x8000 != 0 {
				openVideo()
				return 0
			}
		case VK_S:
			if ctrl&0x8000 != 0 {
				saveVideo()
				return 0
			}
		}
	case WM_COMMAND:
		id := int(loword(wParam))
		code := hiword(wParam)
		if code == EN_KILLFOCUS && (id == 2001 || id == 2002) {
			parseEdits(false)
		}
	case WM_CTLCOLOREDIT:
		procSetBkColor.Call(wParam, EDIT_BG)
		procSetTextColor.Call(wParam, TEXT)
		return state.editBrush
	case WM_TIMER:
		if wParam == TIMER_ID {
			refreshTimeline()
			return 0
		}
	case WM_DROPFILES:
		handleDrop(wParam)
		return 0
	case WM_APP + 1:
		state.mu.Lock()
		state.status = getPendingStatus()
		state.mu.Unlock()
		procInvalidateRect.Call(hwnd, 0, 1)
		return 0
	case WM_APP + 2:
		state.mu.Lock()
		state.progress = getPendingProgress()
		state.mu.Unlock()
		procInvalidateRect.Call(hwnd, 0, 1)
		return 0
	case WM_APP + 3:
		finishExport(getPendingStatus())
		return 0
	case WM_CLOSE:
		stopPlayer()
		stopExport()
		procKillTimer.Call(hwnd, TIMER_ID)
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		if state.editBrush != 0 {
			procDeleteObject.Call(state.editBrush)
		}
		for _, f := range []uintptr{state.fontTitle, state.fontSubtitle, state.fontBody, state.fontSmall, state.fontButton, state.fontMono} {
			if f != 0 {
				procDeleteObject.Call(f)
			}
		}
		procPostQuit.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func updateHover(x, y int32) {
	h := 0
	for id, rr := range map[int]RectF{BTN_OPEN: rectOpen, BTN_PLAY: rectPlay, BTN_RESET: rectReset, BTN_MODE_COPY: RectF{rectModeGroup.x, rectModeGroup.y, rectModeGroup.w / 2, rectModeGroup.h}, BTN_MODE_EXACT: RectF{rectModeGroup.x + rectModeGroup.w/2, rectModeGroup.y, rectModeGroup.w / 2, rectModeGroup.h}, BTN_SAVE: rectSave, BTN_OPEN_FOLDER: rectFolder} {
		if rectContains(rr, x, y) {
			h = id
			break
		}
	}
	if h != state.hover {
		state.hover = h
		procInvalidateRect.Call(state.main, 0, 0)
	}
}

func handleMainClick(x, y int32) {
	state.mu.Lock()
	running := state.exportRunning
	state.mu.Unlock()
	if running {
		return
	}
	switch {
	case rectContains(rectOpen, x, y):
		openVideo()
	case rectContains(rectPlay, x, y):
		togglePlayback()
	case rectContains(rectReset, x, y):
		resetSelection()
	case rectContains(RectF{rectModeGroup.x, rectModeGroup.y, rectModeGroup.w / 2, rectModeGroup.h}, x, y):
		state.mu.Lock()
		state.exactMode = false
		state.mu.Unlock()
		procInvalidateRect.Call(state.main, 0, 1)
	case rectContains(RectF{rectModeGroup.x + rectModeGroup.w/2, rectModeGroup.y, rectModeGroup.w / 2, rectModeGroup.h}, x, y):
		state.mu.Lock()
		state.exactMode = true
		state.mu.Unlock()
		procInvalidateRect.Call(state.main, 0, 1)
	case rectContains(rectSave, x, y):
		saveVideo()
	case rectContains(rectFolder, x, y):
		openOutputFolder()
	}
}

func handleDrop(hdrop uintptr) {
	count, _, _ := procDragQueryFile.Call(hdrop, 0xFFFFFFFF, 0, 0)
	for i := uintptr(0); i < count; i++ {
		buf := make([]uint16, 32768)
		procDragQueryFile.Call(hdrop, i, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		path := syscall.UTF16ToString(buf)
		if strings.EqualFold(filepath.Ext(path), ".mp4") {
			openVideoPath(path)
			break
		}
	}
	procDragFinish.Call(hdrop)
}

func openVideo() {
	path := fileDialog(false, "Открыть MP4", "")
	if path != "" {
		openVideoPath(path)
	}
}

func openVideoPath(path string) {
	state.mu.Lock()
	running := state.exportRunning
	state.mu.Unlock()
	if running {
		return
	}
	path, _ = filepath.Abs(path)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		message("Не удалось открыть выбранный файл.", "Ошибка", MB_ICONERROR|MB_OK)
		return
	}
	if !strings.EqualFold(filepath.Ext(path), ".mp4") {
		message("Поддерживается формат MP4.", "Неподдерживаемый формат", MB_ICONWARNING|MB_OK)
		return
	}
	postStatus("Проверяю видео…")
	m, err := probeVideo(path)
	if err != nil {
		message("Не удалось открыть видео. Возможно, файл повреждён или использует неподдерживаемый кодек.", "Не удалось открыть", MB_ICONERROR|MB_OK)
		return
	}
	d, _ := strconv.ParseFloat(m.Format.Duration, 64)
	if d <= 0 {
		message("У видео не удалось определить длительность.", "Ошибка видео", MB_ICONERROR|MB_OK)
		return
	}
	stopPlayer()
	state.mu.Lock()
	state.currentFile = path
	state.metaLine = metaLineFromMeta(m)
	state.duration = d
	state.start = 0
	state.end = d
	state.current = 0
	state.lastOutput = ""
	state.status = "Видео готово к обрезке"
	state.progress = 0
	state.playing = false
	state.mu.Unlock()
	setText(state.startEdit, formatTime(0))
	setText(state.endEdit, formatTime(d))
	startPlayer(path, 0, true)
	procInvalidateRect.Call(state.main, 0, 1)
	procInvalidateRect.Call(state.preview, 0, 1)
	refreshTimeline()
}

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func probeVideo(path string) (Meta, error) {
	ffprobe := toolPath("ffprobe.exe")
	cmd := exec.Command(ffprobe, "-v", "error", "-show_entries", "format=duration,size:stream=index,codec_name,codec_type,width,height,r_frame_rate", "-of", "json", "--", path)
	hideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(out, &m); err != nil {
		return Meta{}, err
	}
	if m.Format.Duration == "" {
		return Meta{}, fmt.Errorf("missing duration")
	}
	return m, nil
}

func parseFPS(s string) float64 {
	if s == "" {
		return 0
	}
	p := strings.Split(s, "/")
	if len(p) != 2 {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
	a, _ := strconv.ParseFloat(p[0], 64)
	b, _ := strconv.ParseFloat(p[1], 64)
	if b == 0 {
		return 0
	}
	return a / b
}

func parseTimeText(s string) (float64, error) {
	p := strings.Split(strings.TrimSpace(s), ":")
	if len(p) != 3 {
		return 0, fmt.Errorf("time must HH:MM:SS.mmm")
	}
	h, _ := strconv.Atoi(p[0])
	m, _ := strconv.Atoi(p[1])
	sec, _ := strconv.ParseFloat(p[2], 64)
	if h < 0 || m < 0 || m >= 60 || sec < 0 || sec >= 60 {
		return 0, fmt.Errorf("bad time")
	}
	return float64(h*3600+m*60) + sec, nil
}

func formatTime(v float64) string {
	if v < 0 {
		v = 0
	}
	totalMs := int64(v*1000 + 0.5)
	h := totalMs / 3600000
	m := (totalMs % 3600000) / 60000
	s := (totalMs % 60000) / 1000
	ms := totalMs % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

func parseEdits(showErr bool) bool {
	st, e1 := parseTimeText(getText(state.startEdit))
	en, e2 := parseTimeText(getText(state.endEdit))
	state.mu.Lock()
	d := state.duration
	state.mu.Unlock()
	if e1 != nil || e2 != nil || st < 0 || en <= st || en > d {
		if showErr {
			message("Введите корректные значения начала и конца.\n\nКонец должен быть позже начала и не превышать длительность видео.", "Некорректный диапазон", MB_ICONWARNING|MB_OK)
		}
		return false
	}
	state.mu.Lock()
	state.start = st
	state.end = en
	if state.current < st || state.current > en {
		state.current = st
	}
	state.mu.Unlock()
	refreshTimeline()
	return true
}

func resetSelection() {
	state.mu.Lock()
	if state.currentFile == "" {
		state.mu.Unlock()
		return
	}
	d := state.duration
	state.start = 0
	state.end = d
	state.current = 0
	state.mu.Unlock()
	setText(state.startEdit, formatTime(0))
	setText(state.endEdit, formatTime(d))
	seekTo(0)
	refreshTimeline()
}

func refreshTimeline() {
	if state.timeline != 0 {
		procInvalidateRect.Call(state.timeline, 0, 0)
	}
	state.mu.Lock()
	t, d, st, en, playing := state.current, state.duration, state.start, state.end, state.playing
	state.mu.Unlock()
	_ = st
	_ = en
	procInvalidateRect.Call(state.main, 0, 0)
	focus, _, _ := procGetFocus.Call()
	if focus != state.startEdit && focus != state.endEdit {
		setText(state.startEdit, formatTime(st))
		setText(state.endEdit, formatTime(en))
	}
	if d > 0 {
		_ = t / d
	}
	_ = playing
}

func togglePlayback() {
	state.mu.Lock()
	if state.exportRunning {
		state.mu.Unlock()
		return
	}
	f, t := state.currentFile, state.current
	if f == "" {
		state.mu.Unlock()
		return
	}
	was := state.playing
	state.playing = !was
	now := state.playing
	state.mu.Unlock()
	startPlayer(f, t, !now)
	procInvalidateRect.Call(state.main, 0, 0)
}

func seekTo(t float64) {
	state.mu.Lock()
	f := state.currentFile
	playing := state.playing
	if t < 0 {
		t = 0
	}
	if state.duration > 0 && t > state.duration {
		t = state.duration
	}
	state.current = t
	state.mu.Unlock()
	if f != "" {
		startPlayer(f, t, !playing)
	}
	refreshTimeline()
}

func startPlayer(path string, start float64, paused bool) {
	state.playerMu.Lock()
	defer state.playerMu.Unlock()
	stopPlayerLocked()
	ffplay := toolPath("ffplay.exe")
	if _, err := os.Stat(ffplay); err != nil {
		state.mu.Lock()
		state.status = "Предпросмотр недоступен"
		state.playing = false
		state.mu.Unlock()
		procInvalidateRect.Call(state.main, 0, 1)
		return
	}
	windowTitle := fmt.Sprintf("Video Trimmer Preview %d", time.Now().UnixNano())
	args := []string{"-hide_banner", "-loglevel", "error", "-window_title", windowTitle, "-noborder", "-x", "640", "-y", "360", "-ss", formatTime(start), "-autoexit"}
	if paused {
		args = append(args, "-initial_pause", "1")
	}
	args = append(args, "--", path)
	cmd := exec.Command(ffplay, args...)
	hideConsole(cmd)
	if err := cmd.Start(); err != nil {
		state.mu.Lock()
		state.status = "Не удалось запустить предпросмотр"
		state.playing = false
		state.mu.Unlock()
		return
	}
	state.player = cmd
	state.playerWindow = 0
	go func(c *exec.Cmd) {
		err := c.Wait()
		_ = err
		state.playerMu.Lock()
		if state.player == c {
			state.player = nil
			state.playerWindow = 0
			state.mu.Lock()
			state.playing = false
			if state.duration > 0 && state.current > state.duration {
				state.current = state.duration
			}
			state.mu.Unlock()
			procInvalidateRect.Call(state.main, 0, 0)
		}
		state.playerMu.Unlock()
	}(cmd)
	if paused {
		// ffplay does not provide a stable paused-start flag; the editor uses its own play state.
		state.mu.Lock()
		state.playing = false
		state.mu.Unlock()
	}
	go func(title string, c *exec.Cmd) {
		for i := 0; i < 40; i++ {
			time.Sleep(50 * time.Millisecond)
			h, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(utf16Ptr(title))))
			if h != 0 {
				state.playerMu.Lock()
				if state.player == c {
					state.playerWindow = h
					embedPlayerWindow(h)
				}
				state.playerMu.Unlock()
				break
			}
		}
	}(windowTitle, cmd)
}

func embedPlayerWindow(hwnd uintptr) {
	if hwnd == 0 || state.preview == 0 {
		return
	}
	procSetParent.Call(hwnd, state.preview)
	newStyle := uintptr(WS_CHILD_WINDOW | WS_VISIBLE | WS_CLIPSIBLINGS | WS_CLIPCHILDREN)
	procSetWindowLongPtr.Call(hwnd, ^uintptr(15), newStyle)
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE|SWP_FRAMECHANGED|SWP_SHOWWINDOW)
	resizePlayerWindow()
}

func resizePlayerWindow() {
	state.playerMu.Lock()
	h := state.playerWindow
	state.playerMu.Unlock()
	if h == 0 || state.preview == 0 {
		return
	}
	var r RECT
	procGetClientRect.Call(state.preview, uintptr(unsafe.Pointer(&r)))
	if ok, _, _ := procIsWindow.Call(h); ok == 0 {
		return
	}
	procMoveWindow.Call(h, 0, 0, uintptr(r.Right), uintptr(r.Bottom), 1)
}

func stopPlayer() { state.playerMu.Lock(); defer state.playerMu.Unlock(); stopPlayerLocked() }
func stopPlayerLocked() {
	if state.player != nil && state.player.Process != nil {
		_ = state.player.Process.Kill()
	}
	state.player = nil
	state.playerWindow = 0
	procInvalidateRect.Call(state.preview, 0, 1)
}

func timelineWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_PAINT:
		drawTimeline(hwnd)
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_LBUTTONDOWN:
		procSetCapture.Call(hwnd)
		handleTimelineMouse(hwnd, xFromLParam(lParam), true)
		return 0
	case WM_MOUSEMOVE:
		handleTimelineMouse(hwnd, xFromLParam(lParam), false)
		return 0
	case WM_LBUTTONUP:
		state.mu.Lock()
		timelineDrag = 0
		state.mu.Unlock()
		procReleaseCapture.Call()
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

var timelineDrag int

func timelineTimeFromX(hwnd uintptr, x int32) float64 {
	var r RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	w := float64(r.Right - r.Left)
	if w <= 1 {
		return 0
	}
	state.mu.Lock()
	d := state.duration
	state.mu.Unlock()
	if d <= 0 {
		return 0
	}
	pad := 34.0
	q := (float64(x) - pad) / (w - 2*pad)
	if q < 0 {
		q = 0
	}
	if q > 1 {
		q = 1
	}
	return q * d
}

func handleTimelineMouse(hwnd uintptr, x int32, down bool) {
	state.mu.Lock()
	d, st, en := state.duration, state.start, state.end
	state.mu.Unlock()
	if d <= 0 {
		return
	}
	var r RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	w := float64(r.Right - r.Left)
	pad := 34.0
	sx := int32(pad + (w-2*pad)*st/d)
	ex := int32(pad + (w-2*pad)*en/d)
	if down {
		if abs32(x-sx) <= 12 {
			timelineDrag = 1
		} else if abs32(x-ex) <= 12 {
			timelineDrag = 2
		} else {
			timelineDrag = 3
		}
	}
	if timelineDrag == 1 {
		t := timelineTimeFromX(hwnd, x)
		if t >= en-0.05 {
			t = en - 0.05
		}
		if t < 0 {
			t = 0
		}
		state.mu.Lock()
		state.start = t
		state.current = t
		state.mu.Unlock()
		setText(state.startEdit, formatTime(t))
		refreshTimeline()
	}
	if timelineDrag == 2 {
		t := timelineTimeFromX(hwnd, x)
		if t <= st+0.05 {
			t = st + 0.05
		}
		if t > d {
			t = d
		}
		state.mu.Lock()
		state.end = t
		state.current = t
		state.mu.Unlock()
		setText(state.endEdit, formatTime(t))
		refreshTimeline()
	}
	if timelineDrag == 3 {
		t := timelineTimeFromX(hwnd, x)
		state.mu.Lock()
		state.current = t
		playing := state.playing
		f := state.currentFile
		state.mu.Unlock()
		if !playing && f != "" {
			startPlayer(f, t, true)
		}
		refreshTimeline()
	}
}
func abs32(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}

func drawTimeline(hwnd uintptr) {
	var ps PAINTSTRUCT
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var r RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	fill(hdc, r, SURFACE)
	state.mu.Lock()
	d, st, en, cur := state.duration, state.start, state.end, state.current
	state.mu.Unlock()
	drawText(hdc, "ОБРЕЗКА", RECT{26, 14, 120, 34}, MUTED, state.fontSmall, 0)
	drawText(hdc, formatTime(st), RECT{120, 14, 250, 34}, TEXT, state.fontMono, 0)
	drawText(hdc, "—", RECT{250, 14, 270, 34}, MUTED, state.fontSmall, 0)
	drawText(hdc, formatTime(en), RECT{270, 14, 400, 34}, TEXT, state.fontMono, 0)
	drawText(hdc, formatTime(cur)+" / "+formatTime(d), RECT{r.Right - 210, 14, r.Right - 26, 34}, MUTED, state.fontSmall, 0x00000002|DT_SINGLELINE)
	pad := 34.0
	x0 := pad
	x1 := float64(r.Right) - pad
	y := 64.0
	track := RECT{int32(x0), int32(y - 5), int32(x1), int32(y + 5)}
	roundRect(hdc, track, 0x002B3947, 0x002B3947, 6)
	if d > 0 {
		sx := x0 + (x1-x0)*st/d
		ex := x0 + (x1-x0)*en/d
		cx := x0 + (x1-x0)*cur/d
		if ex > sx {
			roundRect(hdc, RECT{int32(sx), int32(y - 5), int32(ex), int32(y + 5)}, ACCENT, ACCENT, 6)
		}
		for i := 0; i <= 4; i++ {
			tx := x0 + (x1-x0)*float64(i)/4
			line(hdc, int32(tx), 78, int32(tx), 85, 0x00333F4C, 1)
		}
		hb := createBrush(TEXT)
		procSelectObject.Call(hdc, hb)
		procEllipse.Call(hdc, uintptr(int32(sx-8)), uintptr(int32(y-13)), uintptr(int32(sx+8)), uintptr(int32(y+13)))
		procEllipse.Call(hdc, uintptr(int32(ex-8)), uintptr(int32(y-13)), uintptr(int32(ex+8)), uintptr(int32(y+13)))
		procDeleteObject.Call(hb)
		cb := createBrush(ACCENT2)
		procSelectObject.Call(hdc, cb)
		procEllipse.Call(hdc, uintptr(int32(cx-5)), uintptr(int32(y-5)), uintptr(int32(cx+5)), uintptr(int32(y+5)))
		procDeleteObject.Call(cb)
	}
}

func saveVideo() {
	state.mu.Lock()
	if state.exportRunning {
		state.mu.Unlock()
		return
	}
	state.mu.Unlock()
	if !parseEdits(true) {
		return
	}
	state.mu.Lock()
	f, st, en, exact := state.currentFile, state.start, state.end, state.exactMode
	state.mu.Unlock()
	if f == "" {
		message("Сначала откройте MP4-видео.", "Нет файла", MB_ICONWARNING|MB_OK)
		return
	}
	name := filepath.Base(strings.TrimSuffix(f, filepath.Ext(f))) + "_cut.mp4"
	out := fileDialog(true, "Сохранить MP4", name)
	if out == "" {
		return
	}
	if !strings.EqualFold(filepath.Ext(out), ".mp4") {
		out += ".mp4"
	}
	if strings.EqualFold(filepath.Clean(out), filepath.Clean(f)) {
		message("Нельзя сохранять поверх исходного файла. Выберите другое имя.", "Выберите другое имя", MB_ICONWARNING|MB_OK)
		return
	}
	state.mu.Lock()
	state.exportRunning = true
	state.lastOutput = out
	state.progress = 0
	state.status = "Обработка видео…"
	state.mu.Unlock()
	procInvalidateRect.Call(state.main, 0, 1)
	go runExport(f, out, st, en, exact)
}

func runExport(input, output string, start, end float64, exact bool) {
	ffmpeg := toolPath("ffmpeg.exe")
	if _, err := os.Stat(ffmpeg); err != nil {
		postFinish("В дистрибутиве отсутствует встроенный FFmpeg.")
		return
	}
	duration := end - start
	if duration <= 0 {
		postFinish("Некорректная длительность фрагмента.")
		return
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	if exact {
		args = append(args, "-i", input, "-ss", formatTime(start), "-t", formatTime(duration), "-map", "0:v:0?", "-map", "0:a?", "-c:v", "libx264", "-preset", "medium", "-crf", "18", "-c:a", "aac", "-b:a", "192k", "-map_metadata", "0", "-movflags", "+faststart")
	} else {
		args = append(args, "-ss", formatTime(start), "-i", input, "-t", formatTime(duration), "-map", "0:v:0?", "-map", "0:a?", "-c:v", "copy", "-c:a", "copy", "-avoid_negative_ts", "make_zero", "-map_metadata", "0")
	}
	tmp := output + ".part.mp4"
	args = append(args, "-progress", "pipe:1", "-nostats", "--", tmp)
	cmd := exec.Command(ffmpeg, args...)
	hideConsole(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		postFinish("Не удалось начать сохранение.")
		return
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		postFinish("Не удалось запустить обработчик видео.")
		return
	}
	state.exportMu.Lock()
	state.exportCmd = cmd
	state.exportMu.Unlock()
	last := 0
	scan := bufio.NewScanner(stdout)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if strings.HasPrefix(line, "out_time_ms=") {
			ms, _ := strconv.ParseInt(strings.TrimPrefix(line, "out_time_ms="), 10, 64)
			sec := float64(ms) / 1e6
			p := int(sec / duration * 100)
			if p > 100 {
				p = 100
			}
			if p > last {
				last = p
				postProgress(p)
			}
		}
	}
	err = cmd.Wait()
	clearExportCmd()
	if err != nil {
		_ = os.Remove(tmp)
		postFinish("Не удалось сохранить видео. Проверьте исходный файл и свободное место.")
		return
	}
	if err := os.Rename(tmp, output); err != nil {
		_ = os.Remove(tmp)
		postFinish("Не удалось завершить сохранение файла.")
		return
	}
	postProgress(100)
	postFinish("Видео успешно сохранено")
	state.mu.Lock()
	state.status = "Видео успешно сохранено"
	state.mu.Unlock()
}

func clearExportCmd() {
	state.exportMu.Lock()
	state.exportCmd = nil
	state.exportMu.Unlock()
}

func stopExport() {
	state.exportMu.Lock()
	cmd := state.exportCmd
	state.exportCmd = nil
	state.exportMu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func finishExport(s string) {
	state.mu.Lock()
	state.exportRunning = false
	if s != "" {
		state.status = s
	}
	if strings.HasPrefix(s, "Видео успешно") {
		state.progress = 100
	} else {
		state.progress = 0
		state.lastOutput = ""
	}
	state.mu.Unlock()
	procInvalidateRect.Call(state.main, 0, 1)
}

func postStatus(s string) {
	pendingMu.Lock()
	pendingStatus = s
	pendingMu.Unlock()
	procPostMessage.Call(state.main, WM_APP+1, 0, 0)
}
func postProgress(p int) {
	pendingMu.Lock()
	pendingProgress = p
	pendingMu.Unlock()
	procPostMessage.Call(state.main, WM_APP+2, 0, 0)
}
func postFinish(s string) {
	pendingMu.Lock()
	pendingStatus = s
	pendingMu.Unlock()
	procPostMessage.Call(state.main, WM_APP+3, 0, 0)
}
func getPendingStatus() string { pendingMu.Lock(); defer pendingMu.Unlock(); return pendingStatus }
func getPendingProgress() int  { pendingMu.Lock(); defer pendingMu.Unlock(); return pendingProgress }

func openOutputFolder() {
	state.mu.Lock()
	out := state.lastOutput
	running := state.exportRunning
	state.mu.Unlock()
	if out == "" || running {
		return
	}
	_ = exec.Command("explorer.exe", "/select,"+filepath.Clean(out)).Start()
}

func fileDialog(save bool, title, defaultName string) string {
	buf := utf16buf(defaultName, 32768)
	filter := utf16buf("MP4 video (*.mp4)\x00*.mp4\x00All files (*.*)\x00*.*\x00", 1024)
	flags := uint32(OFN_EXPLORER | OFN_NOCHANGEDIR | OFN_HIDEREADONLY | OFN_PATHMUSTEXIST)
	if save {
		flags |= OFN_OVERWRITEPROMPT
	} else {
		flags |= OFN_FILEMUSTEXIST
	}
	ofn := OPENFILENAMEW{LStructSize: uint32(unsafe.Sizeof(OPENFILENAMEW{})), HwndOwner: state.main, LpstrFilter: &filter[0], NFilterIndex: 1, LpstrFile: &buf[0], NMaxFile: uint32(len(buf)), LpstrTitle: utf16Ptr(title), Flags: flags, LpstrDefExt: utf16Ptr("mp4")}
	var ok uintptr
	if save {
		ok, _, _ = procChooseSave.Call(uintptr(unsafe.Pointer(&ofn)))
	} else {
		ok, _, _ = procChooseOpen.Call(uintptr(unsafe.Pointer(&ofn)))
	}
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func init() { _ = time.Second }
