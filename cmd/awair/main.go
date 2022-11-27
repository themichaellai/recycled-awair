package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"tinygo.org/x/bluetooth"
)

var adapter = bluetooth.DefaultAdapter

const awairDeviceNme = "AWAIR-R2"
const awairSearchTimeout = time.Second * 15

const awairServiceUUID = "2f2dfff0-2e85-649d-3545-3586428f5da3"
const awairCharacteristicUUID4 = "2f2dfff4-2e85-649d-3545-3586428f5da3"
const awairCharacteristicUUID5 = "2f2dfff5-2e85-649d-3545-3586428f5da3"

func findAwair() (bluetooth.Addresser, error) {
	resCh := make(chan struct {
		Addr bluetooth.Addresser
		Err  error
	})
	go func() {
		if err := adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
			if device.LocalName() == awairDeviceNme {
				resCh <- struct {
					Addr bluetooth.Addresser
					Err  error
				}{device.Address, nil}
			}
		}); err != nil {
			resCh <- struct {
				Addr bluetooth.Addresser
				Err  error
			}{nil, err}
		}
	}()

	select {
	case res := <-resCh:
		if err := adapter.StopScan(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to stop scan: %v", err)
		}
		return res.Addr, res.Err
	case <-time.After(awairSearchTimeout):
		if err := adapter.StopScan(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to stop scan: %v", err)
		}
		return nil, fmt.Errorf("timed out while scanning for awair")
	}
}

func findCharacteristics(device *bluetooth.Device) (char4 bluetooth.DeviceCharacteristic, char5 bluetooth.DeviceCharacteristic, _ error) {
	services, err := device.DiscoverServices(nil)
	if err != nil {
		return bluetooth.DeviceCharacteristic{}, bluetooth.DeviceCharacteristic{}, fmt.Errorf("failed to discover services: %w", err)
	}

	var char4Res *bluetooth.DeviceCharacteristic
	var char5Res *bluetooth.DeviceCharacteristic
	for _, service := range services {
		characteristics, err := service.DiscoverCharacteristics(nil)
		if err != nil {
			return bluetooth.DeviceCharacteristic{}, bluetooth.DeviceCharacteristic{}, fmt.Errorf("failed to discover characteristics: %w", err)
		}
		for _, characteristic := range characteristics {
			//fmt.Printf("service: %s characteristic: %s\n", service.UUID().String(), characteristic.UUID().String())
			if characteristic.UUID().String() == awairCharacteristicUUID4 {
				char4Res = &characteristic
			}
			if characteristic.UUID().String() == awairCharacteristicUUID5 {
				char5Res = &characteristic
			}
		}
	}
	if char4Res == nil || char5Res == nil {
		return bluetooth.DeviceCharacteristic{}, bluetooth.DeviceCharacteristic{}, fmt.Errorf("failed to find all characteristics")
	}
	return *char4Res, *char5Res, nil
}

func run() error {
	if err := adapter.Enable(); err != nil {
		return fmt.Errorf("failed to enable BLE: %w", err)
	}

	fmt.Println("Finding awair...")
	awairAddr, err := findAwair()
	if err != nil {
		return fmt.Errorf("failed to find awair: %w", err)
	}

	fmt.Println("Connecting to awair...")
	device, err := adapter.Connect(awairAddr, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("failed to connect to awair: %w", err)
	}

	fmt.Println("Discovering characteristics...")
	char4, char5, err := findCharacteristics(device)
	if err != nil {
		return fmt.Errorf("failed to find characteristics: %w", err)
	}

	notifyCh := make(chan []byte)
	if err := char4.EnableNotifications(func(data []byte) {
		fmt.Printf("got notification: %s\n", string(data))
		notifyCh <- data
	}); err != nil {
		return fmt.Errorf("failed to enable notifications: %w", err)
	}

	getFwVersionCmd, err := json.Marshal(map[string]interface{}{
		"cmd": "get_fw_version",
	})
	if err != nil {
		return fmt.Errorf("failed to marshal json: %w", err)
	}
	_, err = char5.WriteWithoutResponse(getFwVersionCmd)
	if err != nil {
		return fmt.Errorf("failed to write without response: %w", err)
	}

	var data []byte
	nRead, err := char4.Read(data)
	if err != nil {
		return fmt.Errorf("failed to read: %w", err)
	}
	fmt.Printf("read %d bytes: %s\n", nRead, string(data))

	fmt.Println("sleeping")
	for i := 0; i < 30; i++ {
		fmt.Printf(".")
		select {
		case data := <-notifyCh:
			fmt.Println()
			fmt.Printf("got notification: %s\n", string(data))
		case <-time.After(time.Second):
		}
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}
