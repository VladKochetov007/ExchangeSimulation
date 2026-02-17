package main

import (
	"fmt"
	"os"
	"path/filepath"

	"exchange_sim/logger"
)

func SetupLogDirectories(baseDir string) error {
	dirs := []string{
		baseDir,
		filepath.Join(baseDir, "spot"),
		filepath.Join(baseDir, "perp"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

func CreateLoggers(baseDir string, symbols []string) (map[string]*logger.Logger, *logger.Logger, *os.File, []*os.File, error) {
	generalLogFile, err := os.Create(filepath.Join(baseDir, "general.log"))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create general log file: %w", err)
	}

	generalLogger := logger.New(generalLogFile)
	symbolLoggers := make(map[string]*logger.Logger)
	symbolLogFiles := make([]*os.File, 0, len(symbols))

	for _, symbol := range symbols {
		subdir := "spot"
		if len(symbol) > 4 && symbol[len(symbol)-4:] == "PERP" {
			subdir = "perp"
		}

		safeSymbol := symbol
		for i := 0; i < len(safeSymbol); i++ {
			if safeSymbol[i] == '/' {
				safeSymbol = safeSymbol[:i] + "-" + safeSymbol[i+1:]
			}
		}

		logPath := filepath.Join(baseDir, subdir, safeSymbol+".log")
		logFile, err := os.Create(logPath)
		if err != nil {
			generalLogFile.Close()
			for _, f := range symbolLogFiles {
				f.Close()
			}
			return nil, nil, nil, nil, fmt.Errorf("failed to create log file for %s: %w", symbol, err)
		}
		symbolLogFiles = append(symbolLogFiles, logFile)
		symbolLoggers[symbol] = logger.New(logFile)
	}

	return symbolLoggers, generalLogger, generalLogFile, symbolLogFiles, nil
}
