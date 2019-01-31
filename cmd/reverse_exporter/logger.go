package main

import "go.uber.org/zap"

func prepareLogger(level string) {
	loggerConfig := zap.NewProductionConfig()
	err := loggerConfig.Level.UnmarshalText([]byte(level))
	if err != nil {
		panic(err)
	}

	logger, err := loggerConfig.Build()
	if err != nil {
		panic(err)
	}

	zap.ReplaceGlobals(logger)
}
