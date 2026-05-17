//go:build !linux

package main

func initGPCLK() error {
	return nil
}
