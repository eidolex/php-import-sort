package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Config struct {
	Include              []string `json:"include"`
	Exclude              []string `json:"exclude"`
	Groups               []string `json:"groups"`
	NewlineBetweenGroups bool     `json:"newline_between_groups"`
}

func main() {
	if len(os.Args) > 1 {
		// Single file mode
		filePath := os.Args[1]
		// We need to load config even in single file mode to get groups if available
		// Or we just use default if not found.
		// For now, let's try to load config if it exists, otherwise default.
		config, _ := loadConfig("psort.json")
		if config == nil {
			config = &Config{}
		}
		if err := processFile(filePath, config); err != nil {
			fmt.Printf("Error processing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully sorted imports in %s\n", filePath)
		return
	}

	// Config mode
	config, err := loadConfig("psort.json")
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	var wg sync.WaitGroup
	// Semaphore to limit concurrency (e.g., 100 concurrent files)
	sem := make(chan struct{}, 100)

	err = filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories but check for exclusion first to prune
		if d.IsDir() {
			if shouldExclude(path, config.Exclude) {
				return filepath.SkipDir
			}
			return nil
		}

		if shouldExclude(path, config.Exclude) {
			return nil
		}

		if shouldInclude(path, config.Include) {
			wg.Add(1)
			sem <- struct{}{} // Acquire token
			go func(p string) {
				defer wg.Done()
				defer func() { <-sem }() // Release token

				fmt.Printf("Processing %s...\n", p)
				if err := processFile(p, config); err != nil {
					fmt.Printf("Error processing %s: %v\n", p, err)
				}
			}(path)
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
		os.Exit(1)
	}

	wg.Wait()
}

func loadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

func shouldExclude(path string, patterns []string) bool {
	for _, pattern := range patterns {
		// Check for recursive pattern
		if strings.HasPrefix(pattern, "**/") {
			basePattern := strings.TrimPrefix(pattern, "**/")
			matched, err := filepath.Match(basePattern, filepath.Base(path))
			if err == nil && matched {
				return true
			}
			continue
		}

		// Strict match against full relative path
		matched, err := filepath.Match(pattern, path)
		if err == nil && matched {
			return true
		}

		// Also check if path starts with pattern (directory exclusion)
		// e.g. exclude "vendor" should match "vendor/foo/bar.php"
		if strings.HasPrefix(path, pattern+string(filepath.Separator)) || path == pattern {
			return true
		}
	}
	return false
}

func shouldInclude(path string, patterns []string) bool {
	for _, pattern := range patterns {
		// Check for recursive pattern
		if strings.HasPrefix(pattern, "**/") {
			basePattern := strings.TrimPrefix(pattern, "**/")
			matched, err := filepath.Match(basePattern, filepath.Base(path))
			if err == nil && matched {
				return true
			}
			continue
		}

		// Strict match against full relative path
		matched, err := filepath.Match(pattern, path)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func processFile(filePath string, config *Config) error {
	// Open original file
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get file info to preserve permissions
	info, err := file.Stat()
	if err != nil {
		return err
	}
	mode := info.Mode()

	// Create temp file
	tempFile, err := os.CreateTemp("", "php_sort_*.php")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	// Ensure temp file is cleaned up if we error out before rename
	defer func() {
		tempFile.Close()
		if _, err := os.Stat(tempPath); err == nil {
			os.Remove(tempPath)
		}
	}()

	writer := bufio.NewWriter(tempFile)
	scanner := bufio.NewScanner(file)

	var useBlock []string
	var pendingEmptyLines []string
	inUseBlock := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		isUse := strings.HasPrefix(trimmed, "use ") && strings.HasSuffix(trimmed, ";")
		isEmpty := trimmed == ""

		if isUse {
			if !inUseBlock {
				inUseBlock = true
			}
			// If we were tracking empty lines within a use block, discard them (consolidate)
			pendingEmptyLines = []string{}
			useBlock = append(useBlock, line)
		} else if isEmpty {
			if inUseBlock {
				// Buffer empty lines while in a use block
				pendingEmptyLines = append(pendingEmptyLines, line)
			} else {
				// Not in use block, write immediately
				if _, err := writer.WriteString(line + "\n"); err != nil {
					return err
				}
			}
		} else {
			if inUseBlock {
				// End of use block
				if err := writeSortedBlock(writer, useBlock, config); err != nil {
					return err
				}
				// Write any pending empty lines that came after the last use statement
				for _, emptyLine := range pendingEmptyLines {
					if _, err := writer.WriteString(emptyLine + "\n"); err != nil {
						return err
					}
				}

				useBlock = []string{}
				pendingEmptyLines = []string{}
				inUseBlock = false
			}
			if _, err := writer.WriteString(line + "\n"); err != nil {
				return err
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Flush remaining if file ends with use block
	if inUseBlock {
		if err := writeSortedBlock(writer, useBlock, config); err != nil {
			return err
		}
		// Write any pending empty lines at EOF
		for _, emptyLine := range pendingEmptyLines {
			if _, err := writer.WriteString(emptyLine + "\n"); err != nil {
				return err
			}
		}
	}

	if err := writer.Flush(); err != nil {
		return err
	}

	// Close files before renaming
	tempFile.Close()
	file.Close()

	// Preserve permissions
	if err := os.Chmod(tempPath, mode); err != nil {
		return err
	}

	// Replace original file
	return os.Rename(tempPath, filePath)
}

func writeSortedBlock(w *bufio.Writer, block []string, config *Config) error {
	groups := config.Groups
	sort.Slice(block, func(i, j int) bool {
		lineI := strings.TrimSpace(block[i])
		lineJ := strings.TrimSpace(block[j])

		// Extract import path (remove "use " and ";")
		importI := strings.TrimSuffix(strings.TrimPrefix(lineI, "use "), ";")
		importJ := strings.TrimSuffix(strings.TrimPrefix(lineJ, "use "), ";")

		groupI := getGroupIndex(importI, groups)
		groupJ := getGroupIndex(importJ, groups)

		if groupI != groupJ {
			return groupI < groupJ
		}
		return lineI < lineJ
	})

	var lastGroup int = -1
	if len(block) > 0 {
		lastGroup = getGroupIndex(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(block[0]), "use "), ";"), groups)
	}

	for i, line := range block {
		if i > 0 && len(groups) > 0 {
			currentImport := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(line), "use "), ";")
			currentGroup := getGroupIndex(currentImport, groups)
			if currentGroup != lastGroup {
				if config.NewlineBetweenGroups {
					if _, err := w.WriteString("\n"); err != nil {
						return err
					}
				}
				lastGroup = currentGroup
			}
		}
		if _, err := w.WriteString(line + "\n"); err != nil {
			return err
		}
	}
	return nil
}

func getGroupIndex(importPath string, groups []string) int {
	if len(groups) == 0 {
		return 0
	}
	for i, group := range groups {
		if group == "*" {
			// Check if it matches any OTHER group first?
			// Usually * is the fallback.
			// If we have ["*", "App"], "App\Foo" matches "App". "Vendor\Bar" matches "*".
			// But if we iterate in order:
			// 1. "*" -> Matches everything?
			// If "*" is present, we should probably check specific matches first?
			// Or does order matter? "vendor first" -> ["*", "App"]
			// If I check "*" first, everything matches "*".
			// So "*" should be treated as "matches if nothing else matches".
			continue
		}
		if strings.HasPrefix(importPath, group) {
			return i
		}
	}

	// If we are here, it didn't match any specific group.
	// Find index of "*"
	for i, group := range groups {
		if group == "*" {
			return i
		}
	}

	// If no "*" and no match, put at the end? or beginning?
	// Let's put at the end (max int)
	return len(groups)
}
