package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

func New() (*log.Logger, func(), error) {
	path := os.Getenv("TALKIN_MCP_LOG")
	if path == "" {
		path = filepath.Join("logs", "talkin-mysql-mcp-server.log")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, nil, err
	}

	logger := log.New(io.MultiWriter(file, os.Stderr), "", log.Ldate|log.Ltime|log.Lmicroseconds|log.LUTC)
	closeLogger := func() {
		_ = file.Close()
	}
	return logger, closeLogger, nil
}
