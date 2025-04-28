package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
    "os/exec"
	"runtime"
    "path/filepath"
    "strings"
)

var args []string

func Run(ver string) {
    // init
    version_id = ver
    version_path = ".minecraft/versions/" + ver

    // Check if the version is exist
    json_file, err := os.Open(version_path + "/" + version_id + ".json")
    if err != nil {
        fmt.Fprintln(os.Stderr, "Version not found:", ver)
        return
    }
    defer json_file.Close()

    // Load version json
    json_file_data, err := io.ReadAll(json_file)
    if err != nil {
        fmt.Fprintln(os.Stderr, "Failed to read version json file:", err)
        return
    }
    json.Unmarshal(json_file_data, &versionData)

    // Pass JVM args
    args = append(args,
        "-Xmx1024M",
        "-Xmn128M",
        "-XX:+UseG1GC",
        "-XX:-UseAdaptiveSizePolicy",
        "-XX:-OmitStackTraceInFastThrow",
    )

    if runtime.GOARCH == "386" || runtime.GOARCH == "amd64" {
        args = append(args, "-Xss1M")
    }

    if os_name == "windows" {
        args = append(args, "-XX:HeapDumpPath=MojangTricksIntelDriversForPerformance_javaw.exe_minecraft.exe.heapdump")
        args = append(args, "-Dos.name=Windows 10", "-Dos.version=10.0")
    } else if os_name == "osx" {
        args = append(args, "-XstartOnFirstThread")
    }

    // Pass -D args
    natives_lib_path, _ := filepath.Abs(version_path + "/" + version_id + "-natives")
    args = append(args,
        "-Dminecraft.launcher.brand=Unnamed Minecraft Launcher -Dminecraft.launcher.version=0.0.1",
        "-Djava.library.path=" + natives_lib_path,
    )

    // Pass classpath args
    sep := ":"
    if os_name == "windows" {
        sep = ";"
    }

    var classpaths []string
    lib_path := version_path + "/libraries"
    err = filepath.Walk(lib_path, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        if !info.IsDir() && strings.HasSuffix(path, ".jar") {
            if strings.Contains(filepath.Base(path), "natives-") {
                return nil
            }
            path, _ = filepath.Abs(path)
            classpaths = append(classpaths, path)
        }
        return nil
    })
    if err != nil {
        fmt.Fprintln(os.Stderr, "Failed to walk libraries:", err)
        return
    }
    // Add client jar
    client_jar_path, _ := filepath.Abs(version_path + "/" + version_id + ".jar")
    classpaths = append(classpaths, client_jar_path)
    args = append(args, "-cp", strings.Join(classpaths, sep))
    
    // Pass Minecraft args
    gameDir, _ := filepath.Abs(version_path)
    assetsDir, _ := filepath.Abs(".minecraft/assets")
    args = append(args, "net.minecraft.client.main.Main")
    args = append(args, "--username", "dodo939")
    args = append(args, "--version", version_id)
    args = append(args, "--gameDir", gameDir)
    args = append(args, "--assetsDir", assetsDir)
    args = append(args, "--assetIndex", versionData["assetIndex"].(map[string]any)["id"].(string))
    args = append(args, "--uuid", "00000FFFFFFFFFFFFFFFFFFFFFF76CD9")
    args = append(args, "--accessToken", "00000FFFFFFFFFFFFFFFFFFFFFF76CD9")
    args = append(args, "--userType", "offline")
    args = append(args, "--versionType", "UMCL")

    // Run Minecraft
    os.Chdir(version_path)
    c := exec.Command("java", args...)
    c.Stdout = os.Stdout
    c.Stderr = os.Stderr
    c.Run()
}