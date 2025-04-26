package main

import (
    "fmt"
    "os"
    "github.com/dodo939/unnamed-minecraft-launcher/cmd"
)

func main() {
    if len(os.Args) < 2 {
        fmt.Println("Usage: umcl <command>")
        return
    }

    command := os.Args[1]

    switch command {
        case "install":
            if len(os.Args) < 3 {
                fmt.Println("Usage: umcl install <version>")
                return
            }
            cmd.Install(os.Args[2])
        default:
            fmt.Println("Unknown command:", command)
    }
}
