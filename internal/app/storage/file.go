package storage

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	DefaultDirName = "/home/.peerchat"
)

type File struct {
	file   *os.File
	writer *bufio.Writer
	mu     sync.Mutex
}

func NewFile(filename string) (*File, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("use home dir: %w", err)
	}
	appDir := filepath.Join(homeDir, ".peerchat")
	if err = os.MkdirAll(appDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	logFile := filepath.Join(appDir, filename+".log")
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}

	return &File{
		file:   file,
		writer: bufio.NewWriter(file),
	}, nil
}

func (s *File) SaveMessage(msg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.writer.WriteString(msg + "\n"); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("flush buffer: %w", err)
	}

	return nil
}

func (s *File) Close() error {
	if err := s.writer.Flush(); err != nil {
		return err
	}
	return s.file.Close()
}
