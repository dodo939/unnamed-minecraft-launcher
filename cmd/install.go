package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
)

var os_name string                  // Operating system name ("windows" | "linux" | "osx")
var version_id string               // Version ID
var version_path string             // Path to version json file
var versionData map[string]any      // Version json data
var download_list []internetFile    // List of files need to download
var nativeFiles []nativeFile        // List of native files need to copy

type internetFile struct {
    Path        string
    FullPath    string
    URL         string
}

type nativeFile struct {
    FullPath    string
    Excludes    []any
}

// Remove the filename from the path
func getDir(path string) string {
    for i := len(path) - 1; i >= 0; i-- {
        if path[i] == '/' {
            return path[:i]
        }
    }
    return ""
}

// Download and save the version json file into specified directory
func put_version_json() bool {
    var version_manifest map[string]any

    // Get version manifest
    resp, err := http.Get("https://piston-meta.mojang.com/mc/game/version_manifest.json")
    if err != nil {
        fmt.Fprintln(os.Stderr, "Error:", err)
        return false
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    json.Unmarshal(body, &version_manifest)

    versions := version_manifest["versions"].([]any)
    version_url := ""
    for _, v := range versions {
        version := v.(map[string]any)
        if version["id"] == version_id {
            version_url = version["url"].(string)
            break
        }
    }

    if version_url == "" {
        fmt.Println("Version not found for", version_id)
        return false
    }

    // Get specific version json file
    versionResp, err := http.Get(version_url)
    if err != nil {
        fmt.Fprintln(os.Stderr, "Error:", err)
        return false
    }
    defer versionResp.Body.Close()

    os.MkdirAll(version_path, 0755)
    version_json_file_fd, err := os.Create(version_path + "/" + version_id + ".json")
    if err != nil {
        fmt.Fprintln(os.Stderr, "Error:", err)
        return false
    }
    defer version_json_file_fd.Close()
    
    // Format and save version json file
    versionRespBody, _ := io.ReadAll(versionResp.Body)
    json.Unmarshal(versionRespBody, &versionData)
    formattedVersionRespBody, _ := json.MarshalIndent(versionData, "", "  ")

    _, err = version_json_file_fd.Write(formattedVersionRespBody)
    if err != nil {
        fmt.Fprintln(os.Stderr, "Error:", err)
        return false
    }

    return true
}

// Collect files need to download
func collect_files() bool {
    var assets map[string]any

    // Download and save assets index
    os.MkdirAll(".minecraft/assets/indexes", 0755)
    assetIndex := versionData["assetIndex"].(map[string]any)
    index_id := assetIndex["id"].(string)
    resp, err := http.Get(assetIndex["url"].(string))
    if err != nil {
        fmt.Fprintln(os.Stderr, "Error:", err)
        return false
    }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)

    fd, _ := os.Create(fmt.Sprintf(".minecraft/assets/indexes/%s.json", index_id))
    defer fd.Close()
    fd.Write(body)

    json.Unmarshal(body, &assets)
    assets = assets["objects"].(map[string]any)

    // Add assets files to download list
    for _, obj := range assets {
        hash := obj.(map[string]any)["hash"].(string)
        download_list = append(download_list, internetFile{
            Path: ".minecraft/assets/objects/" + hash[:2],
            FullPath: ".minecraft/assets/objects/" + hash[:2] + "/" + hash,
            URL: "https://resources.download.minecraft.net/" + hash[:2] + "/" + hash,
        })
    }

    // Add client jar to download list
    download_list = append(download_list, internetFile{
        Path: version_path,
        FullPath: version_path + "/" + version_id + ".jar",
        URL: versionData["downloads"].(map[string]any)["client"].(map[string]any)["url"].(string),
    })

    // Add libraries to download list
    for _, lib := range versionData["libraries"].([]any) {
        lib_data := lib.(map[string]any)
        if rules, ok := lib_data["rules"]; ok {
            isAllowed := false
            for _, rule := range rules.([]any) {
                if os_, ok := rule.(map[string]any)["os"].(map[string]any); ok {
                    if os_["name"].(string) == os_name {
                        if rule.(map[string]any)["action"] == "allow" {
                            isAllowed = true
                        } else if rule.(map[string]any)["action"] == "disallow" {
                            isAllowed = false
                        } else {
                            fmt.Fprintln(os.Stderr, "Invalid rule action: " + rule.(map[string]any)["action"].(string))
                            return false
                        }
                    }
                } else {
                    if rule.(map[string]any)["action"] == "allow" {
                        isAllowed = true
                    } else if rule.(map[string]any)["action"] == "disallow" {
                        isAllowed = false
                    } else {
                        fmt.Fprintln(os.Stderr, "Invalid rule action: " + rule.(map[string]any)["action"].(string))
                        return false
                    }
                }
            }
            if !isAllowed {
                continue
            }
        }

        // Add common library to download list
        artifact := lib_data["downloads"].(map[string]any)["artifact"].(map[string]any)
        download_list = append(download_list, internetFile{
            Path: version_path + "/libraries/" + getDir(artifact["path"].(string)),
            FullPath: version_path + "/libraries/" + artifact["path"].(string),
            URL: artifact["url"].(string),
        })

        if natives, ok := lib_data["natives"].(map[string]any); ok {
            if classifier, ok := natives[os_name].(string); ok {
                native_file := lib_data["downloads"].(map[string]any)["classifiers"].(map[string]any)[classifier].(map[string]any)

                // Add native libraries to copy list
                if extract, ok := lib_data["extract"].(map[string]any); ok {
                    if exclude, ok := extract["exclude"].([]any); ok {
                        nativeFiles = append(nativeFiles, nativeFile{
                            FullPath: version_path + "/libraries/" + native_file["path"].(string),
                            Excludes: exclude,
                        })
                    }
                } else {
                    nativeFiles = append(nativeFiles, nativeFile{
                        FullPath: version_path + "/libraries/" + native_file["path"].(string),
                    })
                }

                // Add native libraries to download list
                download_list = append(download_list, internetFile{
                    Path: version_path + "/libraries/" + getDir(native_file["path"].(string)),
                    FullPath: version_path + "/libraries/" + native_file["path"].(string),
                    URL: native_file["url"].(string),
                })
            }
        }
    }

    // Add logging config file to download list
    logging_file := versionData["logging"].(map[string]any)["client"].(map[string]any)["file"].(map[string]any)
    download_list = append(download_list, internetFile{
        Path: version_path,
        FullPath: version_path + "/" + logging_file["id"].(string),
        URL: logging_file["url"].(string),
    })

    return true
}

func download_files() bool {
    // TODO: Download files
    
    return true
}

// Exposed function
func Install(ver string) {
    // Get operating system
    switch runtime.GOOS {
        case "windows":
            os_name = "windows"
        case "linux":
            os_name = "linux"
        case "darwin":
            os_name = "osx"
        default:
            fmt.Fprintln(os.Stderr, "Unsupported operating system")
            return
    }
    // Set version info
    version_id = ver
    version_path = ".minecraft/versions/" + ver

    if !put_version_json() {
        return
    }
    if !collect_files() {
        return
    }
    if !download_files() {
        return
    }

    // Show download_list and nativeFiles
    if false {
        fmt.Println("Download list:")
        for _, file := range download_list {
            fmt.Println(file.URL)
        }
        fmt.Println("Native files:")
        for _, file := range nativeFiles {
            fmt.Println(file.FullPath)
        }
    }
}
