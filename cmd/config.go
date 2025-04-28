package cmd

import (
	"runtime"
	"fmt"
	"os"
)

var os_name         string          // Operating system name ("windows" | "linux" | "osx")
var version_id      string          // Version ID
var version_path    string          // Path to version json file
var versionData     map[string]any  // Version json data

func init() {
    // Get operating system
    switch runtime.GOOS {
    case "windows":
        os_name = "windows"
        fmt.Println("Your operating system is Windows")
    case "linux":
        os_name = "linux"
        fmt.Println("Your operating system is Linux")
    case "darwin":
        os_name = "osx"
        fmt.Println("Your operating system is MacOS")
    default:
        fmt.Fprintln(os.Stderr, "Unsupported operating system")
        os.Exit(1)
    }
}
