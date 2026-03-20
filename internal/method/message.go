package method

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Header struct {
	Key   string
	Value string
}

type Message struct {
	Code    int
	Text    string
	Headers []Header
}

func (m *Message) Get(key string) string {
	for _, h := range m.Headers {
		if h.Key == key {
			return h.Value
		}
	}

	return ""
}

func (m *Message) Set(key, value string) {
	m.Headers = append(m.Headers, Header{Key: key, Value: value})
}

func (m *Message) Write(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "%d %s\n", m.Code, m.Text); err != nil {
		return err
	}

	for _, h := range m.Headers {
		if _, err := fmt.Fprintf(w, "%s: %s\n", h.Key, h.Value); err != nil {
			return err
		}
	}

	_, err := fmt.Fprint(w, "\n")

	return err
}

func ReadMessage(r *bufio.Reader) (*Message, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}

	line = strings.TrimRight(line, "\r\n")

	parts := strings.SplitN(line, " ", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid message line: %s", line)
	}

	code, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid message code: %s", parts[0])
	}

	msg := &Message{
		Code: code,
		Text: parts[1],
	}

	for {
		line, err = r.ReadString('\n')
		if err != nil {
			if err == io.EOF && line == "" {
				break
			}

			if err == io.EOF {
				line = strings.TrimRight(line, "\r\n")
				if line != "" {
					kv := strings.SplitN(line, ": ", 2)
					if len(kv) == 2 {
						msg.Set(kv[0], kv[1])
					}
				}

				break
			}

			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		kv := strings.SplitN(line, ": ", 2)
		if len(kv) == 2 {
			msg.Set(kv[0], kv[1])
		}
	}

	return msg, nil
}
