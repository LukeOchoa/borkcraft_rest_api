package main

import "time"

func session_length() time.Duration {
	var sessionTime = time.Second * 20 //* 90

	return sessionTime
}
