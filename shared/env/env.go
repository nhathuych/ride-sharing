/*
Package env provides a simple way to get environment variables.
*/
package env

import (
	"os"
	"strconv"
	"strings"
)

var (
	Environment = getEnvironment()
	IsProd      = isProd(Environment)
	IsDev       = !IsProd
)

func GetString(key, fallback string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	return val
}

func GetInt(key string, fallback int) int {
	val, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	valAsInt, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}

	return valAsInt
}

func GetBool(key string, fallback bool) bool {
	val, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	boolVal, err := strconv.ParseBool(val)
	if err != nil {
		return fallback
	}

	return boolVal
}

func getEnvironment() string {
	return GetString("ENVIRONMENT", "development")
}

func isProd(environment string) bool {
	environment = strings.ToLower(environment)

	return environment == "production" || environment == "prod"
}
