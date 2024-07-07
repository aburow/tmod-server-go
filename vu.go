package main

import (
    "archive/tar"
    "archive/zip"
    "bufio"
    "compress/gzip"
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "net/http"
    "runtime/pprof"
    "os"
    "path/filepath"
    "strings"
)

// VersionUpdate represents the tModLoader version updater.
type VersionUpdate struct {
    Version            string
    DataDir            string
    ImageURL           string
    RootDir            string
    LogFile            string
    VersionLogFile     string
    InstalledVersion   string
    LatestVersion      string
    CopyConfigFiles    []string
    MoveConfigFiles    []string
    BackupDir          string
    BaseDir            string
}

func NewVersionUpdate() *VersionUpdate {
    return &VersionUpdate{
        InstalledVersion:   "0.0",
        LatestVersion:      "0.0",
        Version:            "0.2",
        DataDir:            ".local/share/Terraria",
        ImageURL:           "https://github.com/tModLoader/tModLoader/releases/latest",
        RootDir:            "/root",
        LogFile:            "/root/tModLoader/tModLoader-Logs/server.log",
        VersionLogFile:     "/root/tModLoader/version_update.json",
        CopyConfigFiles: []string{
                            "/root/tModLoader-v%s/boot_start.sh",
                            "/root/tModLoader-v%s/start.sh",
                            "/root/tModLoader-v%s/serverconfig.txt",
        },
        MoveConfigFiles: []string{
                            "/root/tModLoader/serverconfig.txt",
        },
        BackupDir:          "/root/backup",
        BaseDir:            "/root/tModLoader",

    }
}

func (vu *VersionUpdate) GetLatestVersion() string {
    resp, err := http.Get(vu.ImageURL)
    if err != nil {
        fmt.Println("Error fetching latest version:", err)
        fullstop(1)
    }
    defer resp.Body.Close()

    urlParts := strings.Split(resp.Request.URL.String(), "/")
    urlPart := strings.TrimLeft(urlParts[len(urlParts)-1],"v")

    return urlPart
}

func (vu *VersionUpdate) readFirstLogLine() string {
    file, err := os.Open(vu.LogFile)
    if err != nil {
        fmt.Println("Error opening log file:", err)
        fullstop(1)
    }
    defer file.Close()

    var firstLine string
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        firstLine = scanner.Text()
        break
    }

    if err := scanner.Err(); err != nil {
        fmt.Println("Error reading log file:", err)
        fullstop(1)
    }

    versionParts := strings.Split(strings.Split(firstLine, "+")[1], "|")
    return versionParts[0]
}

func (vu *VersionUpdate) GetInstalledVersion() string {
    version, err := vu.readVersionLog()
    if err != nil {
        fmt.Println("version_update.json not found")
    }

    if version == "" {
        version = vu.readFirstLogLine()
        vu.writeVersionLog(version)
    }

    return version
}

func (vu *VersionUpdate) readVersionLog() (string, error) {
    file, err := os.Open(vu.VersionLogFile)
    if err != nil {
        return "", err
    }
    defer file.Close()

    var versionMap map[string]string
    if err := json.NewDecoder(file).Decode(&versionMap); err != nil {
        return "", err
    }

    return versionMap["version"], nil
}

func (vu *VersionUpdate) writeVersionLog(version string) {
    versionMap := map[string]string{"version": version}
    file, err := os.Create(vu.VersionLogFile)
    if err != nil {
        fmt.Println("Error creating version_update.json:", err)
        fullstop(1)
    }
    defer file.Close()

    encoder := json.NewEncoder(file)
    if err := encoder.Encode(versionMap); err != nil {
        fmt.Println("Error writing version to version_update.json:", err)
        fullstop(1)
    }
}

func (vu *VersionUpdate) BackupExecs() {
    execBackupPath := fmt.Sprintf("%s/tMod-execs-%s.tar.gz", vu.BackupDir, vu.InstalledVersion)
    execBackupDir := fmt.Sprintf("%s/tModLoader", vu.RootDir)

    // Create a tar.gz archive of the current executables
    if err := tarGz(execBackupPath, execBackupDir, vu.BackupDir); err != nil {
        fmt.Println("Error creating executable backup:", err)
        fullstop(1)
    }
}

func (vu *VersionUpdate) BackupDataFiles() {
    dataBackupPath := fmt.Sprintf("%s/tMod-datafiles-%s.tar.gz", vu.BackupDir, vu.InstalledVersion)
    dataBackupDir := fmt.Sprintf("%s/%s", vu.RootDir, vu.DataDir)

    // Create a tar.gz archive of the current data files
    if err := tarGz(dataBackupPath, dataBackupDir, vu.BackupDir); err != nil {
        fmt.Println("Error creating data files backup:", err)
        fullstop(1)
    }
}

func tarGz(outputPath, sourcePath, backupDir string) error {
    // Create the output file
    outputFile, err := os.Create(outputPath)
    if err != nil {
        return err
    }
    defer outputFile.Close()

    // Create a new gzip writer
    gzipWriter := gzip.NewWriter(outputFile)
    defer gzipWriter.Close()

    // Create a new tar writer
    tarWriter := tar.NewWriter(gzipWriter)
    defer tarWriter.Close()

    // Walk through the source directory and add files to the tar archive
    return filepath.Walk(sourcePath, func(filePath string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        // Create a new tar header
        header, err := tar.FileInfoHeader(info, "")
        if err != nil {
            return err
        }

        // Set the header name to the relative path
        header.Name, err = filepath.Rel(sourcePath, filePath)
        if err != nil {
            return err
        }

        // Write the header
        if err := tarWriter.WriteHeader(header); err != nil {
            return err
        }

        // If the file is a regular file, write its contents to the tar archive
        if info.Mode().IsRegular() {
            file, err := os.Open(filePath)
            if err != nil {
                return err
            }
            defer file.Close()

            if _, err := io.Copy(tarWriter, file); err != nil {
                return err
            }
        }

        return nil
    })
}

func (vu *VersionUpdate) MoveCurrentDir() {
    // Rename the "tModLoader" directory to the format "tModLoader-v<installed_version>"
    currentDir := fmt.Sprintf("%s/tModLoader", vu.RootDir)
    newDir := fmt.Sprintf("%s/tModLoader-v%s", vu.RootDir, vu.InstalledVersion)

    if err := os.Rename(currentDir, newDir); err != nil {
        fmt.Println("Error renaming current directory:", err)
        fullstop(1)
    }
}

func (vu *VersionUpdate) MakeNewDir() {
    // Create a new "tModLoader" directory
    newDir := fmt.Sprintf("%s/tModLoader", vu.RootDir)

    if err := os.Mkdir(newDir, os.ModePerm); err != nil {
        fmt.Println("Error creating new directory:", err)
        fullstop(1)
    }
}

func (vu *VersionUpdate) GetLatestZip() {
    // Get the URL for the latest tModLoader version
    zipURL := fmt.Sprintf("https://github.com/tModLoader/tModLoader/releases/download/v%s/tModLoader.zip", vu.LatestVersion)

    // Download the ZIP file
    zipPath := fmt.Sprintf("%s/tModLoader/tModLoader-v%s.zip", vu.RootDir, vu.LatestVersion)
    if err := downloadFile(zipPath, zipURL); err != nil {
        fmt.Println("Error downloading ZIP file:", err)
        fullstop(1)
    }
}

func downloadFile(filePath, url string) error {
    // Create the output file
    output, err := os.Create(filePath)
    if err != nil {
        return err
    }
    defer output.Close()

    // Download the file
    response, err := http.Get(url)
    if err != nil {
        return err
    }
    defer response.Body.Close()

    // Copy the downloaded content to the file
    _, err = io.Copy(output, response.Body)
    return err
}

func (vu *VersionUpdate) ServerSetup() {
    // Unpack the server distribution ZIP
    zipPath := fmt.Sprintf("%s/tModLoader/tModLoader-v%s.zip", vu.RootDir, vu.LatestVersion)
    extractDir := fmt.Sprintf("%s/tModLoader", vu.RootDir)

    if err := unzip(zipPath, extractDir); err != nil {
        fmt.Println("Error unpacking server distribution ZIP:", err)
        fullstop(1)
    }
}

func unzip(zipPath, extractDir string) error {
    // Open the ZIP file
    zipFile, err := zip.OpenReader(zipPath)
    if err != nil {
        return err
    }
    defer zipFile.Close()

    // Extract the contents of the ZIP file
    for _, file := range zipFile.File {
        // Open the file in the ZIP archive
        zipFile, err := file.Open()
        if err != nil {
            return err
        }
        defer zipFile.Close()

        // Create the destination file or directory
        destPath := filepath.Join(extractDir, file.Name)
        if file.FileInfo().IsDir() {
            // If it's a directory, create it
            if err := os.MkdirAll(destPath, os.ModePerm); err != nil {
                return err
            }
        } else {
            // If it's a regular file, create the file
            destFile, err := os.Create(destPath)
            if err != nil {
                return err
            }
            defer destFile.Close()

            // Copy the contents of the file to the destination file
            _, err = io.Copy(destFile, zipFile)
            if err != nil {
                return err
            }
        }
    }

    return nil
}

func (vu *VersionUpdate) DeployStartFiles() {
    // Redeploy config and startup shell files to new directory structure
    for _, file := range vu.MoveConfigFiles {
        originalPath := fmt.Sprintf("%s.orig", file)
        if err := os.Rename(file, originalPath); err != nil {
            fmt.Println("Error moving config file:", err)
            fullstop(1)
        }
    }

    for _, file := range vu.CopyConfigFiles {
        originalPath := fmt.Sprintf(file, vu.InstalledVersion)
        destPath := filepath.Join(fmt.Sprintf("%s/tModLoader", vu.RootDir), filepath.Base(file))
        if err := copyFile(originalPath, destPath); err != nil {
            fmt.Println("Error copying config file:", err)
            fullstop(1)
        }
    }

    // Set execute permission on all *.sh files in the target directory
    shFiles, err := filepath.Glob(filepath.Join(vu.BaseDir, "*.sh"))
    if err != nil {
        fmt.Println("Error finding *.sh files:", err)
        fullstop(1)
    }

    for _, shFile := range shFiles {
        if err := os.Chmod(shFile, 0755); err != nil {
            fmt.Println("Error setting execute permission:", err)
            fullstop(1)
        }
    }

    vu.InstalledVersion = vu.LatestVersion

    // Update version log file
    vu.writeVersionLog(vu.InstalledVersion)
}

func copyFile(src, dest string) error {
    sourceFile, err := os.Open(src)
    if err != nil {
            return err
    }
    defer sourceFile.Close()

    destFile, err := os.Create(dest)
    if err != nil {
            return err
    }
    defer destFile.Close()

    _, err = io.Copy(destFile, sourceFile)
    if err != nil {
            return err
    }

    return nil

}

func (vu *VersionUpdate) main() {

    fmt.Printf("\nLatest Release    : %s\n", vu.LatestVersion)
    fmt.Printf("Installed Release : %s\n", vu.InstalledVersion)
    check := vu.LatestVersion != vu.InstalledVersion
    fmt.Printf("Update Status     : %t\n", check)
    if check {
        fmt.Println("Starting upgrade...")

        // Backup Executables
        fmt.Print("Backup Executables: ")
        vu.BackupExecs()
        fmt.Println("OK")

        // Backup Data Files
        fmt.Print("Backup Data Files : ")
        vu.BackupDataFiles()
        fmt.Println("OK")

        // Move Current Installation
        fmt.Print("Move Current Inst.: ")
        vu.MoveCurrentDir()
        fmt.Println("OK")

        // Prepare Directory
        fmt.Print("Prepare Directory : ")
        vu.MakeNewDir()
        fmt.Println("OK")

        // Retrieve File
        fmt.Print("Retrieve File     : ")
        vu.GetLatestZip()
        fmt.Println("OK")

        // Unzip New Server
        fmt.Print("Unzip New Server  : ")
        vu.ServerSetup()
        fmt.Println("OK")

        // Deploy Start Files
        fmt.Print("Deploy Start Files: ")
        vu.DeployStartFiles()
        fmt.Println("OK")

        fmt.Print("You can now reboot\n\n")
    }

    fmt.Println()
}

func fullstop(dropout int) {
    pprof.StopCPUProfile()
    os.Exit(dropout)
}

// Main Loop
func main() {
    // Your existing command-line options
    checkCommand := flag.NewFlagSet("check", flag.ExitOnError)
    pcheckCommand := flag.NewFlagSet("pcheck", flag.ExitOnError)
    upgradeCommand := flag.NewFlagSet("upgrade", flag.ExitOnError)

    // Create an instance of VersionUpdate
    vu := NewVersionUpdate()

    // Parse the command-line arguments
    flag.Parse()

    // Start profiling to a file if a file is specified
    f, err := os.Create("vu.prof")
    if err != nil {
       fmt.Println("Error creating profile file:", err)
       fullstop(1)
    }
    pprof.StartCPUProfile(f)
    // defer pprof.StopCPUProfile()

    // Check which command is provided
    if flag.NArg() < 1 {
        fmt.Printf("\nUsage: %s <check|upgrade>\n\n", os.Args[0])
        fullstop(2)
    }

    switch os.Args[1] {
    case "check":
        checkCommand.Parse(os.Args[2:])

        installedVersion := vu.GetInstalledVersion()
        latestVersion := vu.GetLatestVersion()

        fmt.Printf("\nInstalled Release : %s\n", installedVersion)
        fmt.Printf("Latest Release    : %s\n", latestVersion)
        updateAvailable := installedVersion != latestVersion
        fmt.Printf("Update Available  : %t\n\n", updateAvailable)

        if updateAvailable {
            fullstop(42)
        } else {
            fullstop(0)
        }

    case "pcheck":
        pcheckCommand.Parse(os.Args[2:])  // Move the parsing after profiling starts

        installedVersion := vu.GetInstalledVersion()
        latestVersion := vu.GetLatestVersion()

        fmt.Printf("\nInstalled Release : %s\n", installedVersion)
        fmt.Printf("Latest Release    : %s\n", latestVersion)
        updateAvailable := installedVersion != latestVersion
        fmt.Printf("Update Available  : %t\n\n", updateAvailable)

        if updateAvailable {
            fullstop(42)
        } else {
            fullstop(0)
        }


    case "upgrade":
        upgradeCommand.Parse(os.Args[2:])

        vu.LatestVersion = vu.GetLatestVersion()          // Populate LatestVersion
        vu.InstalledVersion = vu.GetInstalledVersion()    // Populate InstalledVersion
        vu.main()                                        // Perform the upgrade logic
        fullstop(88)

    default:
        fmt.Printf("\nUsage: %s <check|upgrade>\n\n", os.Args[0])
        fullstop(2)
    }
}
