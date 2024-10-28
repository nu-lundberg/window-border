package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	gdi32                      = syscall.NewLazyDLL("gdi32.dll")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procDestroyWindow          = user32.NewProc("DestroyWindow")
	procDefWindowProcW         = user32.NewProc("DefWindowProcW")
	procRegisterClassExW       = user32.NewProc("RegisterClassExW")
	procGetWindowRect          = user32.NewProc("GetWindowRect")
	procGetForegroundWindow    = user32.NewProc("GetForegroundWindow")
	procUpdateLayeredWindow    = user32.NewProc("UpdateLayeredWindow")
	procGetDC                  = user32.NewProc("GetDC")
	procReleaseDC              = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	procFillRect               = user32.NewProc("FillRect")
	procShowWindow             = user32.NewProc("ShowWindow")
	procPeekMessageW           = user32.NewProc("PeekMessageW")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
)

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

type MSG struct {
	Hwnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type Rect struct {
	Left, Top, Right, Bottom int32
}

type POINT struct {
	X, Y int32
}

type SIZE struct {
	CX, CY int32
}

type BLENDFUNCTION struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

const (
	WS_EX_LAYERED     = 0x00080000
	WS_EX_TOPMOST     = 0x00000008
	WS_EX_TOOLWINDOW  = 0x00000080
	WS_EX_TRANSPARENT = 0x00000020
	WS_POPUP          = 0x80000000
	SW_SHOW           = 5
	THICKNESS         = 5        // Border thickness
	BORDER_COLOR      = 0x0000FF // Red color in hex BGR format
	TRANSPARENCY      = 200      // 0 (transparent) to 255 (opaque)
	ULW_ALPHA         = 0x00000002
	AC_SRC_OVER       = 0x00
	AC_SRC_ALPHA      = 0x01
	PM_REMOVE         = 0x0001
)

type BorderSet struct {
	top, bottom, left, right syscall.Handle
}

func wndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case 0x0010: // WM_CLOSE
		procDestroyWindow.Call(uintptr(hwnd))
		return 0
	default:
		ret, _, _ := procDefWindowProcW.Call(
			uintptr(hwnd),
			uintptr(msg),
			wParam,
			lParam,
		)
		return ret
	}
}

// Register a custom window class
func registerWindowClass(className *uint16) error {
	var wndClass WNDCLASSEXW
	wndClass.CbSize = uint32(unsafe.Sizeof(wndClass))
	wndClass.Style = 0
	wndClass.LpfnWndProc = syscall.NewCallback(wndProc)
	wndClass.HInstance = syscall.Handle(0)
	wndClass.LpszClassName = className

	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wndClass)))
	if atom == 0 {
		return fmt.Errorf("failed to register window class: %v", err)
	}
	return nil
}

// Create a new BorderSet
func newBorderSet(x, y, width, height int) (*BorderSet, error) {
	top, err := createOverlayWindow(x-THICKNESS, y-THICKNESS, width+(2*THICKNESS), THICKNESS)
	if err != nil {
		return nil, err
	}
	bottom, err := createOverlayWindow(x-THICKNESS, y+height, width+(2*THICKNESS), THICKNESS)
	if err != nil {
		destroyWindow(top)
		return nil, err
	}
	left, err := createOverlayWindow(x-THICKNESS, y, THICKNESS, height)
	if err != nil {
		destroyWindow(top, bottom)
		return nil, err
	}
	right, err := createOverlayWindow(x+width, y, THICKNESS, height)
	if err != nil {
		destroyWindow(top, bottom, left)
		return nil, err
	}

	return &BorderSet{top, bottom, left, right}, nil
}

// Destroy the BorderSet
func (b *BorderSet) Destroy() {
	destroyWindow(b.top, b.bottom, b.left, b.right)
	b.top, b.bottom, b.left, b.right = 0, 0, 0, 0 // Reset handles
}

// Helper function to destroy multiple windows
func destroyWindow(handles ...syscall.Handle) {
	for _, h := range handles {
		if h != 0 {
			ret, _, err := procDestroyWindow.Call(uintptr(h))
			if ret == 0 {
				log.Println("Error destroying window:", err)
			}
		}
	}
}

// createOverlayWindow creates an overlay window with transparency and color
func createOverlayWindow(x, y, width, height int) (syscall.Handle, error) {
	className := syscall.StringToUTF16Ptr("MyOverlayWindowClass")
	windowName := syscall.StringToUTF16Ptr("")

	hwnd, _, err := procCreateWindowExW.Call(
		uintptr(WS_EX_LAYERED|WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_TRANSPARENT),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		uintptr(WS_POPUP),
		uintptr(x),
		uintptr(y),
		uintptr(width),
		uintptr(height),
		0,
		0,
		0,
		0,
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("failed to create window: %v", err)
	}

	// Proceed with the rest of the code to set up the window...

	// Get the screen DC
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		procDestroyWindow.Call(hwnd)
		return 0, fmt.Errorf("failed to get screen DC")
	}
	defer procReleaseDC.Call(0, screenDC)

	// Create a memory DC
	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		procDestroyWindow.Call(hwnd)
		return 0, fmt.Errorf("failed to create compatible DC")
	}
	defer procDeleteDC.Call(memDC)

	// Create a bitmap
	hBitmap, _, _ := procCreateCompatibleBitmap.Call(screenDC, uintptr(width), uintptr(height))
	if hBitmap == 0 {
		procDestroyWindow.Call(hwnd)
		return 0, fmt.Errorf("failed to create compatible bitmap")
	}
	defer procDeleteObject.Call(hBitmap)

	// Select the bitmap into the memory DC
	prevBitmap, _, _ := procSelectObject.Call(memDC, hBitmap)
	if prevBitmap == 0 || prevBitmap == ^uintptr(0) {
		procDestroyWindow.Call(hwnd)
		return 0, fmt.Errorf("failed to select bitmap into DC")
	}
	defer procSelectObject.Call(memDC, prevBitmap)

	// Create a solid brush with the desired color
	hBrush, _, _ := procCreateSolidBrush.Call(BORDER_COLOR)
	if hBrush == 0 {
		procDestroyWindow.Call(hwnd)
		return 0, fmt.Errorf("failed to create solid brush")
	}
	defer procDeleteObject.Call(hBrush)

	// Fill the bitmap with the color
	rect := Rect{0, 0, int32(width), int32(height)}
	ret, _, _ := procFillRect.Call(memDC, uintptr(unsafe.Pointer(&rect)), hBrush)
	if ret == 0 {
		procDestroyWindow.Call(hwnd)
		return 0, fmt.Errorf("failed to fill rect")
	}

	// Set up the BLENDFUNCTION
	blend := BLENDFUNCTION{
		BlendOp:             AC_SRC_OVER,
		BlendFlags:          0,
		SourceConstantAlpha: byte(TRANSPARENCY), // 0-255
		AlphaFormat:         0,                  // 0 if no per-pixel alpha
	}

	size := SIZE{int32(width), int32(height)}
	ptSrc := POINT{0, 0}
	ptDst := POINT{int32(x), int32(y)} // Set destination point to the window's position

	ret, _, err = procUpdateLayeredWindow.Call(
		uintptr(hwnd),
		0,                               // hdcDst
		uintptr(unsafe.Pointer(&ptDst)), // pptDst
		uintptr(unsafe.Pointer(&size)),  // psize
		memDC,                           // hdcSrc
		uintptr(unsafe.Pointer(&ptSrc)), // pptSrc
		0,                               // crKey
		uintptr(unsafe.Pointer(&blend)), // pblend
		ULW_ALPHA,                       // dwFlags
	)
	if ret == 0 {
		procDestroyWindow.Call(hwnd)
		return 0, fmt.Errorf("failed to update layered window: %v", err)
	}

	procShowWindow.Call(hwnd, SW_SHOW)
	return syscall.Handle(hwnd), nil
}

// getActiveWindowRect retrieves the handle and dimensions of the active window
func getActiveWindowRect() (syscall.Handle, int, int, int, int, bool) {
	handle, _, _ := procGetForegroundWindow.Call()
	if handle == 0 {
		return 0, 0, 0, 0, 0, false
	}

	var rect Rect
	ret, _, err := procGetWindowRect.Call(handle, uintptr(unsafe.Pointer(&rect)))
	if ret == 0 {
		log.Println("GetWindowRect failed:", err)
		return 0, 0, 0, 0, 0, false
	}

	x := int(rect.Left)
	y := int(rect.Top)
	width := int(rect.Right - rect.Left)
	height := int(rect.Bottom - rect.Top)
	return syscall.Handle(handle), x, y, width, height, true
}

func messageLoop() {
	var msg MSG
	for {
		ret, _, _ := procPeekMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0,
			0,
			0,
			PM_REMOVE,
		)
		if ret != 0 {
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}
}

func main() {
	// Lock the main goroutine to the OS thread
	runtime.LockOSThread()

	// Remove the log file setup
	// Open the log file
	// logFile, err := os.OpenFile("debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	// if err != nil {
	//     fmt.Println("Failed to open log file:", err)
	//     return
	// }
	// defer logFile.Close()

	// Set the output of the default logger to the log file
	// log.SetOutput(logFile)

	// Optionally set the logger to write to stdout
	log.SetOutput(os.Stdout)

	log.Println("Program started")

	// Register the window class
	className := syscall.StringToUTF16Ptr("MyOverlayWindowClass")
	err := registerWindowClass(className)
	if err != nil {
		log.Println("Error registering window class:", err)
		return
	}

	var currentBorders *BorderSet
	var prevHandle syscall.Handle
	var prevX, prevY, prevWidth, prevHeight int

	// Start the message loop in a separate goroutine
	go messageLoop()

	for {
		// Get the current active window and its dimensions
		handle, x, y, width, height, ok := getActiveWindowRect()
		if !ok {
			// Destroy existing borders if any
			if currentBorders != nil {
				currentBorders.Destroy()
				currentBorders = nil
			}
			continue
		}

		// If the window has changed or resized, update borders
		if handle != prevHandle || x != prevX || y != prevY || width != prevWidth || height != prevHeight {
			// Destroy existing borders
			if currentBorders != nil {
				currentBorders.Destroy()
				currentBorders = nil
			}

			// Create new borders
			newBorders, err := newBorderSet(x, y, width, height)
			if err != nil {
				log.Println("Error creating border windows:", err)
				continue
			}

			// Update the current borders and previous window info
			currentBorders = newBorders
			prevHandle, prevX, prevY, prevWidth, prevHeight = handle, x, y, width, height
		}

		// Reduce CPU usage
		time.Sleep(100 * time.Millisecond)
	}
}
