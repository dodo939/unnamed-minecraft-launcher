package util

import (
    "crypto/sha1"
    "encoding/hex"
    "io"
    "os"
)

func CalculateSHA1FromPath(filePath string) (string, error) {
    file, err := os.Open(filePath)
    if err != nil {
        return "", err
    }
    defer file.Close()

    hash := sha1.New()
    if _, err := io.Copy(hash, file); err != nil {
        return "", err
    }

    return hex.EncodeToString(hash.Sum(nil)), nil
}