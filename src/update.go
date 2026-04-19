package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	githubOwner   = "edwinludik"
	githubRepo    = "control_my_server_bot"
	updateDir     = "updates"
	newBinaryName = "control_my_server_bot.new"
)

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func checkForUpdate(currentVersion string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", githubOwner, githubRepo)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status: %s", resp.Status)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	// Remove 'v' prefix from tag name if present
	latestVersion := strings.TrimPrefix(release.TagName, "v")
	if latestVersion != currentVersion {
		return &release, nil
	}

	return nil, nil // Already up to date
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file, status: %s", resp.Status)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, resp.Body)
	return err
}

func verifyChecksum(binaryPath, checksumPath, binaryName string) error {
	checksumData, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("failed to read checksum file: %w", err)
	}

	var expectedChecksum string
	lines := strings.Split(string(checksumData), "\n")
	for _, line := range lines {
		if strings.Contains(line, binaryName) {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				expectedChecksum = parts[0]
				break
			}
		}
	}

	if expectedChecksum == "" {
		return fmt.Errorf("checksum for %s not found in %s", binaryName, checksumPath)
	}

	f, err := os.Open(binaryPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actualChecksum := hex.EncodeToString(h.Sum(nil))
	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	return nil
}

func handleUpdateCommand(chatID int64, logger *TelegramLogger, cfg *Config) {
	release, err := checkForUpdate(cfg.Version)
	if err != nil {
		logger.Printf("Failed to check for updates: %v", err)
		logger.SendMessage(chatID, "Failed to check for updates: "+err.Error())
		return
	}

	if release == nil {
		logger.SendMessage(chatID, "Bot is already up to date (version "+cfg.Version+").")
		return
	}

	msgText := fmt.Sprintf("A new version is available: *%s*\nCurrent version: *%s*\n\nDo you want to update?", release.TagName, cfg.Version)
	logger.SendMarkdown(chatID, msgText)
}

func performUpdate(chatID int64, logger *TelegramLogger, cfg *Config) {
	release, err := checkForUpdate(cfg.Version)
	if err != nil || release == nil {
		logger.Printf("Failed to re-verify update before performing: %v", err)
		logger.SendMessage(chatID, "Failed to re-verify update.")
		return
	}

	// 1. Identify current binary path
	selfPath, err := os.Executable()
	if err != nil {
		logger.Printf("Failed to get executable path: %v", err)
		logger.SendMessage(chatID, "Failed to get executable path.")
		return
	}
	dir := filepath.Dir(selfPath)
	absUpdateDir := filepath.Join(dir, updateDir)

	// 2. Prepare update directory
	if err := os.MkdirAll(absUpdateDir, 0755); err != nil {
		logger.Printf("Failed to create update directory %s: %v", absUpdateDir, err)
		logger.SendMessage(chatID, "Failed to prepare update directory.")
		return
	}

	// 3. Find assets (binary and checksums)
	binaryURL, checksumURL := getAssetURLs(release)

	if binaryURL == "" || checksumURL == "" {
		logger.Printf("Could not find suitable assets in release %s (Binary: %v, Checksum: %v)", release.TagName, binaryURL != "", checksumURL != "")
		logger.SendMessage(chatID, "Could not find suitable assets in the release (binary or checksums.txt missing).")
		return
	}

	logger.SendMessage(chatID, "Downloading update...")

	tmpBinary := filepath.Join(absUpdateDir, newBinaryName)
	tmpChecksum := filepath.Join(absUpdateDir, "checksums.txt")

	if err := downloadFile(binaryURL, tmpBinary); err != nil {
		logger.Printf("Failed to download binary: %v", err)
		logger.SendMessage(chatID, "Failed to download binary.")
		return
	}

	if err := downloadFile(checksumURL, tmpChecksum); err != nil {
		logger.Printf("Failed to download checksums: %v", err)
		logger.SendMessage(chatID, "Failed to download checksums.")
		return
	}

	// 4. Verify checksum
	binaryNameInAsset := filepath.Base(binaryURL)
	if err := verifyChecksum(tmpBinary, tmpChecksum, binaryNameInAsset); err != nil {
		logger.Printf("Checksum verification failed: %v", err)
		_ = os.Remove(tmpBinary)
		_ = os.Remove(tmpChecksum)
		logger.SendMessage(chatID, "Checksum verification failed.")
		return
	}

	// Cleanup checksum file as it's no longer needed
	_ = os.Remove(tmpChecksum)

	logger.SendMessage(chatID, "✅ Update downloaded successfully. Restarting to apply...")
	logger.Printf("🔄 Update downloaded. Version %s -> %s. Restarting service to apply.", cfg.Version, release.TagName)

	// 5. Restart the service
	go restartService()
}

func getAssetURLs(release *GitHubRelease) (binaryURL, checksumURL string) {
	for _, asset := range release.Assets {
		switch asset.Name {
		case "checksums.txt":
			checksumURL = asset.BrowserDownloadURL
		case "control_my_server_bot":
			binaryURL = asset.BrowserDownloadURL
		}
	}
	return
}

func restartService() {
	log.Println("Self-restarting via systemctl...")
	cmd := exec.Command("sudo", "systemctl", "restart", "control_my_server_bot")
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to trigger restart: %v", err)
		os.Exit(0)
	}
}
