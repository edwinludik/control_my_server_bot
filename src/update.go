package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	latestReleaseURL = "https://api.github.com/repos/edwinludik/control_my_server_bot/releases/latest"
	newBinaryName    = "control_my_server_bot.new"
	checksumFileName = "checksums.txt"
)

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func checkForUpdate(currentVersion string) (*GitHubRelease, error) {
	resp, err := http.Get(latestReleaseURL)
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

func downloadFile(urlStr string, root *os.Root, fileName string) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	// Validation: Only allow HTTPS and trusted GitHub hosts
	if u.Scheme != "https" {
		return fmt.Errorf("unsupported protocol: %s", u.Scheme)
	}
	if u.Host != "github.com" && u.Host != "objects.githubusercontent.com" {
		return fmt.Errorf("untrusted host: %s", u.Host)
	}

	// #nosec G107
	resp, err := http.Get(urlStr)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file, status: %s", resp.Status)
	}

	out, err := root.Create(fileName)
	if err != nil {
		return err
	}

	if _, err = io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		return err
	}
	// Close explicitly before reopening for chmod
	_ = out.Close()

	// Ensure the binary is executable
	f, err := root.Open(fileName)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if info, err := f.Stat(); err == nil {
		// Only chmod if it is likely the binary (has no extension or is the bot's name)
		if !strings.Contains(fileName, ".") {
			if err := f.Chmod(0700); err != nil {
				log.Printf("Warning: failed to chmod new binary: %v", err)
			}
		}
		_ = info
	}

	return nil
}

func verifyChecksum(root *os.Root, binaryFileName, checksumFileName, originalBinaryName string) error {
	checksumData, err := root.ReadFile(checksumFileName)
	if err != nil {
		return fmt.Errorf("failed to read checksum file: %w", err)
	}

	var expectedChecksum string
	lines := strings.Split(string(checksumData), "\n")
	for _, line := range lines {
		if strings.Contains(line, originalBinaryName) {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				expectedChecksum = parts[0]
				break
			}
		}
	}

	if expectedChecksum == "" {
		return fmt.Errorf("checksum for %s not found in %s", originalBinaryName, checksumFileName)
	}

	f, err := root.Open(binaryFileName)
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

	msgText := fmt.Sprintf("A new version is available: %s\nCurrent version: %s\n\nDo you want to update?", release.TagName, cfg.Version)
	msg := tgbotapi.NewMessage(chatID, msgText)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Yes, update", "confirm_update"),
			tgbotapi.NewInlineKeyboardButtonData("No, cancel", "close_message"),
		),
	)
	logger.Send(msg)
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

	// Open the application directory as a secure root (Go 1.24+)
	root, err := os.OpenRoot(dir)
	if err != nil {
		logger.Printf("Failed to open root directory %s: %v", dir, err)
		logger.SendMessage(chatID, "Failed to prepare secure update environment.")
		return
	}
	defer func() { _ = root.Close() }()

	// 3. Find assets (binary and checksums)
	binaryURL, checksumURL := getAssetURLs(release)

	if binaryURL == "" || checksumURL == "" {
		logger.Printf("Could not find suitable assets in release %s (Binary: %v, Checksum: %v)", release.TagName, binaryURL != "", checksumURL != "")
		logger.SendMessage(chatID, "Could not find suitable assets in the release (binary or checksums.txt missing).")
		return
	}

	logger.SendMessage(chatID, "Downloading update...")

	if err := downloadFile(binaryURL, root, newBinaryName); err != nil {
		logger.Printf("Failed to download binary: %v", err)
		logger.SendMessage(chatID, "Failed to download binary.")
		return
	}

	if err := downloadFile(checksumURL, root, checksumFileName); err != nil {
		logger.Printf("Failed to download checksums: %v", err)
		logger.SendMessage(chatID, "Failed to download checksums.")
		return
	}

	// 4. Verify checksum
	binaryNameInAsset := filepath.Base(binaryURL)
	if err := verifyChecksum(root, newBinaryName, checksumFileName, binaryNameInAsset); err != nil {
		logger.Printf("Checksum verification failed: %v", err)
		_ = root.Remove(newBinaryName)
		_ = root.Remove(checksumFileName)
		logger.SendMessage(chatID, "Checksum verification failed.")
		return
	}

	// Cleanup checksum file as it's no longer needed
	_ = root.Remove(checksumFileName)

	logger.SendMessage(chatID, "✅ Update downloaded successfully. Restarting to apply...")
	logger.Printf("Update downloaded. Version %s -> %s. Restarting service to apply.", cfg.Version, release.TagName)

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
