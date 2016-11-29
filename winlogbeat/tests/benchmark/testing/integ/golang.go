package integ

import (
	"flag"
	"fmt"
	"runtime"
	"os"
	"path/filepath"
)

const (
	downloadBase = "https://storage.googleapis.com/golang"
)

var (
	goBaseURL   = flag.String("go-baseurl", downloadBase, "Download base URL for the Go distribution")
	goVersion   = flag.String("go-version", "1.7.3", "Go version to download")
	installGo  = flag.Bool("install-go", false, "Download and install Go in a temporary directory")
)

// SetupGolang returns the GOROOT for a Go installation. If -install-go=true
// then this will download and install Go to a temporary directory.
func SetupGolang(destinationDir string) (string, error) {
	if *installGo {
		file, err := downloadGo(*goVersion, runtime.GOOS, runtime.GOARCH, destinationDir)
		if err != nil {
			return "", err
		}
		defer os.Remove(file)

		err = Extract(file, destinationDir)
		if err != nil {
			return "", err
		}

		return filepath.Join(destinationDir, "go"), nil
	}

	// Use GOROOT from environment.
	goroot := os.Getenv("GOROOT")
	if goroot == "" {
		return "", fmt.Errorf("GOROOT is not set in environment")
	}

	// Test if Go exists at the GOPATH.
	var ext string
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	_, err := os.Stat(filepath.Join(goroot, "bin", "go"+ext))
	if err != nil {
		return "", err
	}

	return goroot, nil
}

// downloadGo downloads the Golang package over HTTPS.
func downloadGo(version, goos, arch, destinationDir string) (string, error) {
	//	Example: https://storage.googleapis.com/golang/go1.7.3.windows-amd64.zip
	extension := "tar.gz"
	if goos == "windows" {
		extension = "zip"
	}

	goURL := fmt.Sprintf("%s/go%v.%v-%v.%v", *goBaseURL, version, goos, arch, extension)
	return DownloadFile(goURL, destinationDir)
}
