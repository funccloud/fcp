package config

import (
	"path/filepath"

	"k8s.io/client-go/util/homedir"
)

func GetConfigDir() string {
	home := homedir.HomeDir()
	return filepath.Join(home, ".fcp")
}
