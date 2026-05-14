package respond

import (
	"testing"
)

func TestConfigMapstructureTags(t *testing.T) {
	CheckMapstructureTags(t, Config{}, "")
}
