package main

// version is the build version, injectable at compile time via:
//
//	go build -ldflags "-X main.version=v0.1.0"
var version = "dev"
