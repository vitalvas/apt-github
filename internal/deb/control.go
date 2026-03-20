package deb

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
)

type Control struct {
	Fields []Field
}

type Field struct {
	Key   string
	Value string
}

func (c *Control) Get(key string) string {
	for _, f := range c.Fields {
		if f.Key == key {
			return f.Value
		}
	}

	return ""
}

func ParseControl(data []byte) (*Control, error) {
	controlTar, err := extractControlTar(data)
	if err != nil {
		return nil, fmt.Errorf("failed to extract control archive: %w", err)
	}

	controlFile, err := extractControlFile(controlTar)
	if err != nil {
		return nil, fmt.Errorf("failed to extract control file: %w", err)
	}

	return parseControlFields(controlFile)
}

func extractControlTar(data []byte) ([]byte, error) {
	reader := bytes.NewReader(data)

	sig := make([]byte, 8)
	if _, err := io.ReadFull(reader, sig); err != nil {
		return nil, fmt.Errorf("failed to read ar signature: %w", err)
	}

	if string(sig) != "!<arch>\n" {
		return nil, fmt.Errorf("not a valid ar archive")
	}

	for {
		header := make([]byte, 60)

		_, err := io.ReadFull(reader, header)
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("failed to read ar header: %w", err)
		}

		name := strings.TrimRight(string(header[0:16]), " /")
		sizeStr := strings.TrimSpace(string(header[48:58]))

		var size int64
		if _, err := fmt.Sscanf(sizeStr, "%d", &size); err != nil {
			return nil, fmt.Errorf("failed to parse ar entry size: %w", err)
		}

		if strings.HasPrefix(name, "control.tar") {
			content := make([]byte, size)
			if _, err := io.ReadFull(reader, content); err != nil {
				return nil, fmt.Errorf("failed to read control archive: %w", err)
			}

			return content, nil
		}

		if _, err := reader.Seek(size, io.SeekCurrent); err != nil {
			return nil, err
		}

		if size%2 != 0 {
			if _, err := reader.Seek(1, io.SeekCurrent); err != nil {
				return nil, err
			}
		}
	}

	return nil, fmt.Errorf("control archive not found")
}

func extractControlFile(controlTar []byte) ([]byte, error) {
	gzReader, err := gzip.NewReader(bytes.NewReader(controlTar))
	if err != nil {
		return nil, fmt.Errorf("failed to decompress control archive: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}

		name := strings.TrimPrefix(header.Name, "./")
		if name == "control" {
			content, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read control file: %w", err)
			}

			return content, nil
		}
	}

	return nil, fmt.Errorf("control file not found in archive")
}

func parseControlFields(data []byte) (*Control, error) {
	ctrl := &Control{}
	scanner := bufio.NewScanner(bytes.NewReader(data))

	var currentKey string
	var currentValue strings.Builder

	flush := func() {
		if currentKey != "" && currentValue.Len() > 0 {
			ctrl.Fields = append(ctrl.Fields, Field{
				Key:   currentKey,
				Value: currentValue.String(),
			})
		}

		currentKey = ""
		currentValue.Reset()
	}

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			flush()
			continue
		}

		if line[0] == ' ' || line[0] == '\t' {
			if currentKey != "" {
				currentValue.WriteString("\n")
				currentValue.WriteString(line)
			}

			continue
		}

		flush()

		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 {
			currentKey = parts[0]
			currentValue.WriteString(parts[1])
		}
	}

	flush()

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return ctrl, nil
}
