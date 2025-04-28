package cmd

import (
    "archive/zip"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "strings"
    "sync"
    "time"

    "github.com/dodo939/unnamed-minecraft-launcher/util"
)

var download_list []internetFile // List of files need to download
var nativeFiles []nativeFile     // List of native files need to copy

var loading_chars = []rune{'|', '/', '-', '\\'}
var loading_index = 0

// Variables for download
var (
    wg         sync.WaitGroup
    semaphore  chan struct{}
    totalFiles int
    completed  int
)

type internetFile struct {
    Path        string
    FullPath    string
    URL         string
    SHA1        string
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
    fmt.Println("Getting version manifest...")
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
        fmt.Fprintln(os.Stderr, "Version not found for", version_id)
        return false
    }

    // Get specific version json file
    fmt.Println("Getting version json file...")
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
    fmt.Println("Collecting asset files...")
    for _, obj := range assets {
        hash := obj.(map[string]any)["hash"].(string)
        download_list = append(download_list, internetFile{
            Path: ".minecraft/assets/objects/" + hash[:2],
            FullPath: ".minecraft/assets/objects/" + hash[:2] + "/" + hash,
            URL: "https://resources.download.minecraft.net/" + hash[:2] + "/" + hash,
            SHA1: hash,
        })
    }

    // Add client jar to download list
    download_list = append(download_list, internetFile{
        Path: version_path,
        FullPath: version_path + "/" + version_id + ".jar",
        URL: versionData["downloads"].(map[string]any)["client"].(map[string]any)["url"].(string),
        SHA1: versionData["downloads"].(map[string]any)["client"].(map[string]any)["sha1"].(string),
    })

    // Add libraries to download list
    fmt.Println("Collecting library files...")
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
            SHA1: artifact["sha1"].(string),
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
                    SHA1: native_file["sha1"].(string),
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
        SHA1: logging_file["sha1"].(string),
    })

    return true
}

func download_all_files() bool {
    fmt.Println("Download start")

    totalFiles = len(download_list)
    semaphore = make(chan struct{}, 8)  // Limit concurrency to 8
    for _, file := range download_list {
        wg.Add(1)
        go download_single_file(file)
    }

    go show_progress()

    wg.Wait()
    fmt.Printf("\nDownload completed for all files!\n")

    return true
}

func extract_native_files() bool {
    fmt.Println("Extracting native files...")

    output_dir := fmt.Sprintf(".minecraft/versions/%s/%s-natives", version_id, version_id)
    os.MkdirAll(output_dir, 0755)

    for _, file := range nativeFiles {
        if err := extract_single_jar(file.FullPath, output_dir, file.Excludes); err != nil {
            fmt.Fprintf(os.Stderr, "Failed to extract %s: %v\n", file.FullPath, err)
            return false
        }
    }

    return true
}

// Exposed function
func Install(ver string) {
    // Set version info
    version_id = ver
    version_path = ".minecraft/versions/" + ver

    if !put_version_json() {
        return
    }
    if !collect_files() {
        return
    }
    if !download_all_files() {
        return
    }
    if !extract_native_files() {
        return
    }

    fmt.Printf("✨ Installation completed for %s! ✨\n", version_id)
}

func download_single_file(file internetFile) {
    defer wg.Done()

    semaphore <- struct{}{}
    defer func() { <-semaphore }()

    var err error
    for range 3 {
        err = download_and_save_file(file)
        if err == nil {
            break
        }
        fmt.Printf("Error downloading %s: %v. Retrying...\n", file.URL, err)
        time.Sleep(1 * time.Second)
    }

    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to download %s after 3 attempts: %v\n", file.URL, err)
        return
    }

    completed++
}

func download_and_save_file(file internetFile) error {
    if _, err := os.Stat(file.FullPath); err == nil {
        sha1, err := util.CalculateSHA1FromPath(file.FullPath)
        if err == nil && sha1 == file.SHA1 {
            return nil // File already exists and has correct SHA1
        }
    }

    resp, err := http.Get(file.URL)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("bad status: %s", resp.Status)
    }

    os.MkdirAll(file.Path, 0755)
    out, err := os.Create(file.FullPath)
    if err != nil {
        return err
    }
    defer out.Close()

    _, err = io.Copy(out, resp.Body)
    return err
}

func show_progress() {
    ticker := time.NewTicker(200 * time.Millisecond)
    defer ticker.Stop()

    for range ticker.C {
        fmt.Printf("\rDownloading %d/%d files... %c", completed, totalFiles, loading_chars[loading_index])
        loading_index = (loading_index + 1) % 4

        if completed == totalFiles {
            return
        }
    }
}

func extract_single_jar(fullpath, output_dir string, excludes []any) error {
    r, err := zip.OpenReader(fullpath)
    if err != nil {
        return fmt.Errorf("failed to open jar file: %w", err)
    }
    defer r.Close()

    for _, f := range r.File {
        if should_exclude(f.Name, excludes) {
            continue
        }

        target_path := output_dir + "/" + f.Name

        if f.FileInfo().IsDir() {
            os.MkdirAll(target_path, f.Mode())
            continue
        }

        os.MkdirAll(getDir(target_path), 0755)

        rc, err := f.Open()
        if err != nil {
            return fmt.Errorf("failed to open file %s in jar: %w", f.Name, err)
        }

        outFile, err := os.OpenFile(target_path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
        if err != nil {
            rc.Close()
            return fmt.Errorf("failed to create file %s: %w", target_path, err)
        }

        _, err = io.Copy(outFile, rc)
        rc.Close()
        outFile.Close()

        if err != nil {
            return fmt.Errorf("failed to write file %s: %w", target_path, err)
        }
    }

    return nil
}

func should_exclude(path string, excludes []any) bool {
    for _, exclude := range excludes {
        exclude_path := exclude.(string)
        if strings.HasPrefix(path, exclude_path) || path == exclude_path {
            return true
        }
    }

    return false
}
