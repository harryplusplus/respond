package respond

import (
	"log/slog"
	"os"
)

func RunServer() error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	// if config.APIURL == "" {
	// 	return fmt.Errorf("api_url is required in config file")
	// }

	// if config.APIKeyEnv != "" {
	// 	apiKey := os.Getenv(config.APIKeyEnv)
	// 	if apiKey == "" {
	// 		// slog.Warn("api key not found", "env", config.APIKeyEnv)
	// 		// TODO: error
	// 	} else {
	// 		slog.Info("api key loaded", "env", config.APIKeyEnv)
	// 	}
	// }

	// slog.Info("Starting server", "host", config.Host, "port", config.Port, "api_url", config.APIURL)
	return nil
}
