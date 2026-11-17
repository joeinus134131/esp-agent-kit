package main

import (
	"errors"
	"strings"

	"go.bug.st/serial/enumerator"
)

const (
	devVID = "303A" // Espressif. Ganti dengan VID/PID produkmu saat dijual
	devPID = "1001" // PID bawaan USB Serial/JTAG ESP32-C3
)

var errNoDevice = errors.New("perangkat tidak ditemukan")

func findPort() (string, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return "", err
	}
	for _, p := range ports {
		if p.IsUSB && strings.EqualFold(p.VID, devVID) && strings.EqualFold(p.PID, devPID) {
			return p.Name, nil
		}
	}
	return "", errNoDevice
}
