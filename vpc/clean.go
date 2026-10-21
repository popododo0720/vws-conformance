package vpc

import "strings"

func ignoreGone(err error) error {
	if err == nil || strings.Contains(err.Error(), "NotFound") {
		return nil
	}
	return err
}
