/*
 * Chouf
 * author: Nicolas CARPi
 * copyright: 2022
 * license: MIT
 * repo: https://github.com/deltablot/chouf
 */

package main

import (
	"log"
	"os"
	"strconv"
	"time"
)

// retrieve a boolean value from an env key, return false if not found or error converting value
func GetBoolEnv(key string) bool {
	if value, found := os.LookupEnv(key); found {
		if converted, err := strconv.ParseBool(value); err == nil {
			return converted
		}
	}
	return false
}

// retrieve a Duration from env
func GetDurationEnv(key string, fallback time.Duration) time.Duration {
	envVal := GetStrEnv(key, "5m")
	duration, err := time.ParseDuration(envVal)
	if err == nil {
		return duration
	}
	return fallback
}

// get an int from env
func GetIntEnv(key string, fallback int) int {
	value, found := os.LookupEnv(key)
	if found {
		converted, err := strconv.Atoi(value)
		if err == nil {
			return converted
		}
	}
	return fallback
}

// get a string from env if it exists
func GetStrEnv(key string, fallback string) string {
	if value, found := os.LookupEnv(key); found {
		return value
	}
	return fallback
}

// print a debug message only if debug mode is set
func Debug(msg string) {
	if app.debug {
		log.Println("chouf: debug:", msg)
	}
}
