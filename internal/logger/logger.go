package logger

import (
	"fmt"
	"log"
	"os"
	"time"
)

// ANSI — hoạt động trên Windows Terminal, Linux, Mac.
// Render (Linux) hỗ trợ đầy đủ.
const (
	colorReset  = "\033[0m"
	colorGray   = "\033[90m"
	colorCyan   = "\033[36m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorBold   = "\033[1m"
	colorGreen  = "\033[32m"
)

var out = log.New(os.Stdout, "", 0)

func ts() string {
	return colorGray + time.Now().Format("15:04:05") + colorReset
}

// Info — khởi động, kết nối thành công, sự kiện thông thường.
func Info(tag, format string, args ...any) {
	out.Printf("%s  %sINFO%s  [%s] %s", ts(), colorCyan, colorReset, tag, fmt.Sprintf(format, args...))
}

// Warn — sự kiện không nghiêm trọng nhưng cần chú ý.
func Warn(tag, format string, args ...any) {
	out.Printf("%s  %sWARN%s  [%s] %s", ts(), colorYellow, colorReset, tag, fmt.Sprintf(format, args...))
}

// Error — lỗi đã được xử lý, chương trình tiếp tục chạy.
func Error(tag, format string, args ...any) {
	out.Printf("%s %sERROR%s  [%s] %s", ts(), colorRed, colorReset, tag, fmt.Sprintf(format, args...))
}

// Fatal — lỗi không thể khắc phục, dừng chương trình.
func Fatal(tag, format string, args ...any) {
	out.Printf("%s %sFATAL%s  [%s] %s", ts(), colorRed+colorBold, colorReset, tag, fmt.Sprintf(format, args...))
	os.Exit(1)
}

// OK — bước khởi động hoàn tất (xanh lá).
func OK(tag, format string, args ...any) {
	out.Printf("%s    %sOK%s  [%s] %s", ts(), colorGreen, colorReset, tag, fmt.Sprintf(format, args...))
}

// Banner in khối thông tin khi bot khởi động.
func Banner(version string) {
	fmt.Printf("\n%s%s", colorBold, colorCyan)
	fmt.Println("  ╔══════════════════════════════════╗")
	fmt.Println("  ║     Discord Casino Bot v" + version + "      ║")
	fmt.Println("  ╚══════════════════════════════════╝")
	fmt.Printf("%s\n", colorReset)
}
