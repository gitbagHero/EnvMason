package executable

import (
	"errors"
	"strings"
)

func isSymlinkLoop(err error) bool {
	if errors.Is(err, errSymlinkLoop) || isPlatformSymlinkLoop(err) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "too many") && strings.Contains(message, "link")
}
