package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Flicster/peerchat/internal/app/model"
)

const (
	fileExtension = ".msg.log"
)

type File struct {
	filename string
	file     *os.File
	writer   *bufio.Writer
	mu       sync.Mutex
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
	logFile := filepath.Join(appDir, filename+fileExtension)
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}

	return &File{
		filename: logFile,
		file:     file,
		writer:   bufio.NewWriter(file),
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

func (s *File) LoadMessages() ([]model.ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.filename)
	if err != nil {
		return nil, fmt.Errorf("open for reading: %w", err)
	}
	defer file.Close()

	result := make([]model.ChatMessage, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg model.ChatMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		result = append(result, msg)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan file: %w", err)
	}
	return result, nil
}

func (s *File) Close() error {
	if err := s.writer.Flush(); err != nil {
		return err
	}
	return s.file.Close()
}
