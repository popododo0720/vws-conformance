package s3

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}

func closeChecked(c io.Closer) {
	if c == nil {
		return
	}
	if err := c.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close: %v\n", err)
	}
}

func xmlTag(body []byte, tag string) string {
	text := string(body)
	open := "<" + tag + ">"
	end := "</" + tag + ">"
	i := strings.Index(text, open)
	if i < 0 {
		return ""
	}
	rest := text[i+len(open):]
	j := strings.Index(rest, end)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func bucketName(rtName string) string {
	return strings.ToLower(rtName)
}
